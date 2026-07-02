# The dn-tool package, shared by the flake's packages.default and the overlay.
# version is injected by the flake (git describe); the overlay leaves it "dev".
{
  buildGoModule,
  version ? "dev",
}:

buildGoModule {
  pname = "dn-tool";
  inherit version;

  # The Go module lives under src/, not the repo root.
  src = ../src;
  vendorHash = "sha256-3E79DpWwCqn+oKM0Y3TkV0Hx1AhwQmXIPuUoTQQQvHU=";

  env.CGO_ENABLED = 0;

  # Mirror the Makefile ldflags so `dn-tool --version` reports the stamped
  # version rather than the "dev" build-info fallback.
  ldflags = [
    "-s"
    "-w"
    "-X github.com/andreswebs/dn-tool/internal/version.Override=${version}"
  ];

  meta = {
    description = "Control-plane CLI for defined.net Managed Nebula host enrollment";
    homepage = "https://github.com/andreswebs/dn-tool";
    mainProgram = "dn-tool";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      "x86_64-darwin"
      "aarch64-darwin"
    ];
  };
}
