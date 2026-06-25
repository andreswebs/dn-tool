# NixOS VM test for services.dnclient (design §2.7/§2.11). It validates the
# module's systemd integration — not dn-tool's own logic, which the binary's
# unit/integration suites cover. Both dn-tool and dnclient are faked so the test
# is hermetic (no network, no proprietary binary):
#
#   - unit ordering + enroll-on-boot,
#   - the dnclient daemon coming up under Type=notify,
#   - the reboot-vs-poweroff unenroll discrimination (the §2.7 spike): the
#     poweroff-only unit must be wired to poweroff/halt but not reboot, must not
#     run on a normal boot, and must not run across a reboot.
{
  system,
  nixpkgs,
  dnTool,
}:

let
  pkgs = nixpkgs.legacyPackages.${system};

  # Fake dnclient: honors `run` under Type=notify (notify ready from the main
  # PID, then block), and ignores enroll (dn-tool drives enrollment here).
  fakeDnclient = pkgs.writeShellScript "dnclient" ''
    case "''${1:-}" in
      run) ${pkgs.systemd}/bin/systemd-notify --ready; exec ${pkgs.coreutils}/bin/sleep infinity ;;
      *) exit 0 ;;
    esac
  '';

  # Fake dn-tool: install places the fake dnclient; enroll writes the local
  # config dn-tool's host-id reader expects and a marker; unenroll removes the
  # config and drops a marker. Markers let the test observe which units ran.
  fakeDnTool = pkgs.writeShellApplication {
    name = "dn-tool";
    runtimeInputs = [ pkgs.coreutils ];
    # The DN_* config comes from the unit environment, so ShellCheck sees these
    # as referenced-but-unassigned (SC2154); that is by design here.
    excludeShellChecks = [ "SC2154" ];
    text = ''
      cmd="''${1:-}"
      case "$cmd" in
        install)
          mkdir -p "$DN_CLIENT_BIN_DIR"
          install -m 0755 ${fakeDnclient} "$DN_CLIENT_BIN_DIR/dnclient"
          ;;
        enroll)
          dir="$DN_CLIENT_CONFIG_DIR/$DN_NETWORK_NAME"
          mkdir -p "$dir"
          printf 'metadata:\n  host_id: host-test\n' > "$dir/dnclient.yml"
          touch /var/lib/defined/enroll.ran
          ;;
        unenroll)
          rm -rf "''${DN_CLIENT_CONFIG_DIR:?}/$DN_NETWORK_NAME"
          touch /var/lib/defined/unenroll.ran
          ;;
        *)
          echo "fake dn-tool: unknown command: $cmd" >&2
          exit 64
          ;;
      esac
    '';
  };

  environmentFile = pkgs.writeText "dn-config.env" "DN_API_KEY=test-key\n";
in
pkgs.testers.runNixOSTest {
  name = "dnclient-module";

  nodes.machine = {
    imports = [ dnTool.nixosModules.dnclient ];
    services.dnclient = {
      enable = true;
      package = fakeDnTool;
      inherit environmentFile;
      network = {
        networkId = "network-test";
        roleId = "role-test";
      };
    };
  };

  testScript = ''
    start_all()

    # Boot ordering: install -> daemon -> enroll, each reaching its active state.
    machine.wait_for_unit("dnclient-install.service")
    machine.wait_for_unit("dnclient@defined.service")
    machine.wait_for_unit("dnclient-enroll.service")

    # install placed the (fake) dnclient binary; enroll wrote local config.
    machine.succeed("test -x /var/lib/defined/bin/dnclient")
    machine.succeed("test -f /var/lib/defined/enroll.ran")
    machine.succeed("test -f /var/lib/defined/defined/dnclient.yml")

    # The poweroff-only unenroll unit must NOT have run on a normal boot.
    machine.fail("test -f /var/lib/defined/unenroll.ran")
    machine.fail("systemctl is-active --quiet dnclient-unenroll.service")

    # ...and it must be pulled in by poweroff/halt but not reboot (§2.7).
    wanted = machine.succeed("systemctl show dnclient-unenroll.service -p WantedBy")
    assert "poweroff.target" in wanted, wanted
    assert "halt.target" in wanted, wanted
    assert "reboot.target" not in wanted, wanted

    # A reboot keeps the host enrolled: unenroll must not fire, config persists.
    machine.reboot()
    machine.wait_for_unit("dnclient-enroll.service")
    machine.fail("test -f /var/lib/defined/unenroll.ran")
    machine.succeed("test -f /var/lib/defined/defined/dnclient.yml")
  '';
}
