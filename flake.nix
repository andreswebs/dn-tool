{
  description = "dn-tool — control-plane CLI for defined.net Managed Nebula host enrollment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      # Buildable on aarch64-darwin for development; the real deployment targets
      # are x86_64-linux / aarch64-linux (design §2.1).
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      # Mirror the Makefile's `git describe --dirty --always`: the short commit
      # when the tree is clean, a dirty marker otherwise, "dev" when out of git.
      version = self.shortRev or self.dirtyShortRev or "dev";
    in
    {
      # The package, built against the consumer's own nixpkgs. Consumers apply
      # this so the services.dnclient module's `package` default (pkgs.dn-tool)
      # resolves without a second nixpkgs in their closure (design §2.1).
      overlays.default = import ./nix/overlay.nix;

      # NixOS integration: systemd units, firewall, config plumbing (design
      # §2.7). The module's package default is pkgs.dn-tool, so a consumer must
      # also apply overlays.default (or set services.dnclient.package).
      nixosModules.dnclient = import ./nix/module.nix;
      nixosModules.default = self.nixosModules.dnclient;

      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.callPackage ./nix/package.nix { inherit version; };
        }
      );

      checks = forAllSystems (
        system:
        nixpkgs.lib.optionalAttrs (nixpkgs.lib.hasSuffix "-linux" system) {
          # NixOS VM test for the services.dnclient module (design §2.7/§2.11):
          # unit ordering, enroll-on-boot, and the reboot-vs-poweroff unenroll
          # discrimination — driven against a fake API and fake dnclient so it
          # stays hermetic.
          module = import ./nix/tests/module.nix {
            inherit system nixpkgs;
            dnTool = self;
          };
        }
      );
    };
}
