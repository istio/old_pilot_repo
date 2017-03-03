# Build instructions for Linux and Mac

We are using [Bazel 0.4.4](https://bazel.io) to build Istio Manager:

    bazel build //cmd/manager

Bazel uses `BUILD` files to keep track of dependencies between sources.  If you
add a new source file or change the imports, please run the following command
to update all `BUILD` files:

    gazelle -go_prefix "istio.io/manager" --mode fix -repo_root .

Gazelle binary is located in `bazel-bin/external` folder under the manager
repository, after your initial bazel build:

    bazel-bin/external/io_bazel_rules_go_repository_tools/bin/gazelle

_Note_: If you cant find the gazelle binary in the path mentioned above,
try to update the mlocate database and run `locate gazelle`. The gazelle
binary should typically be in

    $HOME/.cache/bazel/_bazel_<username>/<somelongfoldername>/external/io_bazel_rules_go_repository_tools/bin/gazelle
