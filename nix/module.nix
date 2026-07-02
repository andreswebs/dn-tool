{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.dnclient;
  net = cfg.network;
  client = cfg.client;

  pkg = cfg.package;
  dnTool = lib.getExe pkg;
  dnclientBin = "${client.binDir}/dnclient";

  boolStr = b: if b then "true" else "false";
  needsListenPort = net.isLighthouse || net.isRelay;

  instance = "dnclient@${net.networkName}.service";

  # Non-secret configuration, safe to keep in the world-readable Nix store. The
  # secret DN_API_KEY (and any DN_* overrides) is supplied separately via
  # cfg.environmentFile, never here.
  envLines =
    [
      "DN_NETWORK_ID=${net.networkId}"
      "DN_ROLE_ID=${net.roleId}"
      "DN_NETWORK_NAME=${net.networkName}"
      "DN_API_URL=${net.apiUrl}"
      "DN_CLIENT_BIN_DIR=${client.binDir}"
      "DN_CLIENT_CONFIG_DIR=${client.configDir}"
      "DN_IS_LIGHTHOUSE=${boolStr net.isLighthouse}"
      "DN_IS_RELAY=${boolStr net.isRelay}"
      "DN_LOG_LEVEL=${cfg.logLevel}"
    ]
    ++ lib.optional (net.apiTimeout != null) "DN_API_TIMEOUT=${net.apiTimeout}"
    ++ lib.optional (net.ipAddress != null) "DN_IP_ADDRESS=${net.ipAddress}"
    ++ lib.optional (net.hostname != null) "DN_HOSTNAME=${net.hostname}"
    ++ lib.optional (net.tags != [ ]) "DN_TAGS='${builtins.toJSON net.tags}'"
    ++ lib.optional (net.staticAddresses != [ ]) "DN_STATIC_ADDRESSES='${builtins.toJSON net.staticAddresses}'"
    ++ lib.optional (net.listenPort != null) "DN_LISTEN_PORT=${toString net.listenPort}"
    ++ lib.optional (client.version != null) "DN_CLIENT_VERSION=${client.version}";

  nonSecretEnvFile = pkgs.writeText "dnclient.env" (lib.concatStringsSep "\n" envLines + "\n");

  # Sandboxing shared by the dn-tool control-plane units. dn-tool needs outbound
  # network (REST API), read of the api-key file, and read/write of the state
  # tree (binary + dnclient config). It needs no elevated capabilities — those
  # belong to the daemon — so the whole capability set is dropped (SEC4).
  controlPlaneHardening = {
    ProtectSystem = "strict";
    ProtectHome = true;
    PrivateTmp = true;
    NoNewPrivileges = true;
    ProtectKernelTunables = true;
    ProtectControlGroups = true;
    RestrictNamespaces = true;
    RestrictRealtime = true;
    LockPersonality = true;
    ReadWritePaths = [ "/var/lib/defined" ];
    CapabilityBoundingSet = "";
    AmbientCapabilities = "";
    RestrictAddressFamilies = [
      "AF_INET"
      "AF_INET6"
      "AF_UNIX"
    ];
    SystemCallFilter = [ "@system-service" ];
    SystemCallErrorNumber = "EPERM";
  };

  # Sandboxing for the proprietary dnclient daemon. Unlike the control plane it
  # genuinely needs CAP_NET_ADMIN and /dev/net/tun to create and configure the
  # Nebula tun device, and AF_NETLINK to program routes — so those are granted
  # and everything else is constrained (SEC4). Validated against the runtime
  # arrangement proven in the repo's docker compose (root + NET_ADMIN + tun).
  daemonHardening = {
    ProtectSystem = "strict";
    ProtectHome = true;
    PrivateTmp = true;
    NoNewPrivileges = true;
    ProtectKernelTunables = true;
    ProtectControlGroups = true;
    LockPersonality = true;
    ReadWritePaths = [ "/var/lib/defined" ];
    CapabilityBoundingSet = [ "CAP_NET_ADMIN" ];
    AmbientCapabilities = [ "CAP_NET_ADMIN" ];
    DeviceAllow = [ "/dev/net/tun rw" ];
    RestrictAddressFamilies = [
      "AF_INET"
      "AF_INET6"
      "AF_UNIX"
      "AF_NETLINK"
    ];
  };

  # Targets that pull in the poweroff-only unenroll unit. By default only
  # poweroff/halt unenroll the host; reboot keeps it enrolled (the §2.5 invariant
  # makes a reboot resume on existing credentials). unenrollOnReboot opts reboot
  # in as well.
  shutdownTargets = [
    "poweroff.target"
    "halt.target"
  ]
  ++ lib.optional net.unenrollOnReboot "reboot.target";
in
{
  options.services.dnclient = {
    enable = lib.mkEnableOption "defined.net dnclient (managed Nebula) via dn-tool";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.dn-tool;
      defaultText = lib.literalExpression "pkgs.dn-tool";
      description = "The dn-tool package providing the control-plane binary.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.path;
      description = ''
        Path to a root-only (0600) systemd EnvironmentFile, wired into the enroll
        and unenroll units. It must define `DN_API_KEY=...` and may define any
        other `DN_*` variables (e.g. `DN_NETWORK_ID`, `DN_ROLE_ID`,
        `DN_STATIC_ADDRESSES`) to override the non-secret values rendered from the
        options below — it is sourced AFTER the in-store env file, so its values
        win. This file must never enter the Nix store; provision it out of band
        (sops-nix, Terraform, cloud-init, or manually). The API key needs the
        `hosts:create`, `hosts:list`, `hosts:enroll`, and `hosts:delete` scopes.
      '';
    };

    logLevel = lib.mkOption {
      type = lib.types.str;
      default = "info";
      description = "dn-tool log level (DN_LOG_LEVEL).";
    };

    network = {
      networkName = lib.mkOption {
        type = lib.types.str;
        default = "defined";
        description = ''
          Network name (DN_NETWORK_NAME). Determines the dnclient instance
          (dnclient@<name>.service), the tun device, and the config subdirectory
          under the dnclient config root.
        '';
      };

      networkId = lib.mkOption {
        type = lib.types.str;
        description = "defined.net network ID to enroll into (DN_NETWORK_ID).";
      };

      roleId = lib.mkOption {
        type = lib.types.str;
        description = "defined.net role ID to assign to the host (DN_ROLE_ID).";
      };

      apiUrl = lib.mkOption {
        type = lib.types.str;
        default = "https://api.defined.net";
        description = "defined.net REST API base URL (DN_API_URL). Override for staging.";
      };

      apiTimeout = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "30s";
        description = ''
          Per-command API deadline (DN_API_TIMEOUT), a Go duration string. Must
          stay below the unenroll unit's TimeoutStartSec so the unenroll DELETE
          completes before systemd kills it (finding D5).
        '';
      };

      ipAddress = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = "Static Nebula IP address for the host (DN_IP_ADDRESS).";
      };

      hostname = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        description = "Enrollment display name (DN_HOSTNAME). Defaults to the system hostname.";
      };

      tags = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
        example = [
          "role:server"
          "env:prod"
        ];
        description = "Tags assigned on enrollment (DN_TAGS).";
      };

      isLighthouse = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Enroll as a lighthouse. Requires staticAddresses and a non-zero listenPort.";
      };

      isRelay = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = "Enroll as a relay. Requires a non-zero listenPort.";
      };

      staticAddresses = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
        example = [ "203.0.113.10:4242" ];
        description = "Public IP:port pairs advertised for lighthouse discovery (DN_STATIC_ADDRESSES).";
      };

      listenPort = lib.mkOption {
        type = lib.types.nullOr lib.types.port;
        default = null;
        description = ''
          UDP port the host listens on (DN_LISTEN_PORT; null lets the system
          choose). Required and non-zero for lighthouses and relays.
        '';
      };

      skipUnenroll = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = ''
          Never unenroll (delete the host) on shutdown. Omits the poweroff
          unenroll unit entirely. Use for persistent hosts such as lighthouses.
        '';
      };

      unenrollOnReboot = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = ''
          Unenroll on reboot as well as poweroff/halt. By default the host stays
          enrolled across reboots and is unenrolled only on poweroff/halt.
        '';
      };

      forceReenroll = lib.mkOption {
        type = lib.types.bool;
        default = false;
        description = ''
          Add --force to the boot-time enroll, auto-healing an enroll-path orphan
          (a remote record with no local config; design §2.4/§2.5). Safe only
          where same-name host collisions are impossible.
        '';
      };

      openFirewall = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Open listenPort (UDP) in the firewall when listenPort is set.";
      };
    };

    client = {
      binDir = lib.mkOption {
        type = lib.types.str;
        default = "/var/lib/defined/bin";
        description = "Directory dn-tool installs the dnclient binary into (DN_CLIENT_BIN_DIR).";
      };

      configDir = lib.mkOption {
        type = lib.types.str;
        default = "/var/lib/defined";
        description = ''
          dnclient config root (DN_CLIENT_CONFIG_DIR); the per-network
          <name>/dnclient.yml lives here. dn-tool passes no -config path to
          dnclient, so this must match dnclient's own built-in default.
        '';
      };

      version = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "0.9.5";
        description = ''
          dnclient version to install (DN_CLIENT_VERSION). null tracks the
          downloads API's `latest`. dn-tool always verifies the binary against
          the published checksum regardless of this pin.
        '';
      };
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = !(net.isLighthouse && net.isRelay);
        message = "services.dnclient: a host cannot be both a lighthouse and a relay.";
      }
      {
        assertion = net.isLighthouse -> net.staticAddresses != [ ];
        message = "services.dnclient: a lighthouse requires network.staticAddresses.";
      }
      {
        assertion = needsListenPort -> (net.listenPort != null && net.listenPort != 0);
        message = "services.dnclient: lighthouses and relays require a non-zero network.listenPort.";
      }
    ];

    environment.systemPackages = [ pkg ];

    systemd.tmpfiles.rules = [
      "d /var/lib/defined 0750 root root -"
      "d ${client.binDir} 0750 root root -"
    ];

    networking.firewall.allowedUDPPorts = lib.optionals (
      net.openFirewall && net.listenPort != null
    ) [ net.listenPort ];

    # (1) Download + verify the dnclient binary at runtime (no API key needed:
    # the downloads endpoint is unauthenticated).
    systemd.services."dnclient-install" = {
      description = "Download and verify the defined.net dnclient binary";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        EnvironmentFile = [ nonSecretEnvFile ];
        ExecStart = "${dnTool} install";
      }
      // controlPlaneHardening;
    };

    # (2) The proprietary dnclient daemon, instanced by network name. The API URL
    # is baked from net.apiUrl (dnclient takes it as a flag, not from env).
    # Fix B1: a StartLimitIntervalSec window wide enough that StartLimitBurst is
    # actually reachable (the upstream 5s/120s/10 combination can never trip).
    systemd.services."dnclient@" = {
      description = "defined.net dnclient (%i)";
      after = [
        "network-online.target"
        "dnclient-install.service"
      ];
      wants = [ "network-online.target" ];
      requires = [ "dnclient-install.service" ];
      startLimitIntervalSec = 1200;
      startLimitBurst = 10;
      unitConfig.ConditionFileIsExecutable = dnclientBin;
      serviceConfig = {
        Type = "notify";
        NotifyAccess = "main";
        ExecStart = ''${dnclientBin} run -server ${net.apiUrl} -name "%i"'';
        Restart = "always";
        RestartSec = 120;
      }
      // daemonHardening;
    };

    # Pull in the concrete instance for the configured network at boot.
    systemd.targets.multi-user.wants = [ instance ];

    # (3) Enroll on boot. Fix B2: bind the *concrete* dnclient@<name>.service (the
    # upstream bare `dnclient.service` reference matched no unit). No ExecStop —
    # unenroll is a separate poweroff-only unit (below), so a `nixos-rebuild
    # switch` unit restart or a reboot never churns enrollment.
    systemd.services."dnclient-enroll" = {
      description = "Enroll this host in defined.net";
      after = [
        "network-online.target"
        instance
      ];
      wants = [ "network-online.target" ];
      requires = [ instance ];
      wantedBy = [ "multi-user.target" ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        EnvironmentFile = [
          nonSecretEnvFile
          cfg.environmentFile
        ];
        ExecStart = "${dnTool} enroll" + lib.optionalString net.forceReenroll " --force";
      }
      // controlPlaneHardening;
    };

    # (4) Unenroll on poweroff/halt only. The work runs in ExecStart, and the unit
    # is pulled into the transaction solely by the poweroff/halt (and optionally
    # reboot) targets it is WantedBy — so it never activates during normal
    # operation, `systemctl restart`, `nixos-rebuild switch`, or (by default)
    # reboot. DefaultDependencies=no keeps it out of the normal shutdown ordering
    # except for the explicit Before= below, which orders it ahead of the target
    # so the DELETE runs while the network is still up. skipUnenroll omits it
    # entirely. TimeoutStartSec bounds the API call (finding D5); the §2.5
    # invariant makes a failed best-effort call safe (local config retained, host
    # resumes next boot).
    systemd.services."dnclient-unenroll" = lib.mkIf (!net.skipUnenroll) {
      description = "Unenroll this host from defined.net on poweroff";
      after = [
        "network-online.target"
        instance
      ];
      before = shutdownTargets;
      wantedBy = shutdownTargets;
      unitConfig.DefaultDependencies = false;
      serviceConfig = {
        Type = "oneshot";
        EnvironmentFile = [
          nonSecretEnvFile
          cfg.environmentFile
        ];
        ExecStart = "${dnTool} unenroll";
        TimeoutStartSec = 60;
      }
      // controlPlaneHardening;
    };
  };
}
