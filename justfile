# Recipes
@default:
  just --list

setup: build install

build *ARGS:
  go build {{ ARGS }}

install:
  chmod +x sshrc && \
  sudo install sshrc /usr/local/bin

gh-release VERSION *ARGS:
  gh release create v{{ VERSION }} --generate-notes {{ ARGS }} './sshrc'
