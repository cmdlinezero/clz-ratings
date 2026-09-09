with import <nixpkgs> {};

let
  # External Script: This reads './dev.kdl' and puts it into a binary named 'dev-zellij'
  scriptZellijLayout = pkgs.writeText "dev.kdl" (builtins.readFile ./dev.kdl);
  zellijLayout = pkgs.writeShellScriptBin "dev-zellij" ''
    ${pkgs.zellij}/bin/zellij --layout ${scriptZellijLayout}
  '';
in
pkgs.mkShell {
  name = "go-dev";

  nativeBuildInputs = with pkgs; [
    go 
    tree
    zellij                         # zellij support
    zellijLayout                   # zellij layout definition
  ];

  LANGUAGE     = "Go";
  VERSION      = "go version";

  shellHook = ''
    # Optional: Script environment start up 
    echo "Welcome to $LANGUAGE Development Environment"
    $VERSION
    # Set a layout using Zellij
    exec dev-zellij
  '';
}
