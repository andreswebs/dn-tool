{
  lib,
  buildGoModule,
  version ? "dev",
}:

buildGoModule {
  pname = "dn-tool";
  inherit version;

  src = ../src;
  vendorHash = "sha256-3E79DpWwCqn+oKM0Y3TkV0Hx1AhwQmXIPuUoTQQQvHU=";

  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X github.com/andreswebs/dn-tool/internal/version.Override=${version}"
  ];

  meta = {
    description = "Control-plane CLI for defined.net Managed Nebula host enrollment";
    homepage = "https://github.com/andreswebs/dn-tool";
    license = lib.licenses.unlicense;
    maintainers = [
      {
        name = "Andre Silva";
        github = "andreswebs";
        githubId = 30079182;
      }
    ];

    mainProgram = "dn-tool";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      ## The dn-tool itself builds on darwin, but the proprietary daemon from defined.net doesn't support it
      # "x86_64-darwin"
      # "aarch64-darwin"
    ];
  };
}
