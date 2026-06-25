# nixpkgs overlay adding `dn-tool` to a consumer's package set, built against the
# consumer's own nixpkgs. Consumers apply this (e.g. nixpkgs.overlays) so the
# services.dnclient module's `package` default (pkgs.dn-tool) resolves without
# pulling a second nixpkgs into their closure.
final: prev: {
  dn-tool = final.callPackage ./package.nix { };
}
