{
  description = "lastro development shell";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      # Everything `task` needs except Docker, which `task db` and `task image`
      # expect from the host.
      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            golangci-lint
            sqlc
            go-task
            nodejs_22
            postgresql_17 # psql against the compose database
          ];

          # Use the Go from this shell instead of downloading the toolchain go.mod names.
          GOTOOLCHAIN = "local";
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
