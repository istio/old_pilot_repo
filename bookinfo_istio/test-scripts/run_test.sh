hub="docker.io/kimikowang"

SCRIPTDIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
WORKSPACE=$SCRIPTDIR/../..
BINDIR=$WORKSPACE/bazel-bin
REGDIR=$SCRIPTDIR/../registry
APPSDIR=$SCRIPTDIR/../apps
BUILDPATH=$SCRIPTDIR/../docker
PILOTSRCPATH=$WORKSPACE/cmd/pilot

# Build the pilot binary
cd $PILOTSRCPATH && bazel build :pilot
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: build failed for pilot.\n***********\n"
    exit $STATUS
fi

# Build the app binaries
set -x
set -o errexit

#docker push $hub

# Copy the pilot binary to each app dir
# Build the images and  push them to hub
for app in details productpage ratings reviews; do
  rm -rf $APPSDIR/$app/pilot && cp $BINDIR/cmd/pilot/pilot $_  
  make -C $APPSDIR/$app build build.dockerize docker.push clean GOOS=linux GOARCH=amd64 HUB=$hub
done

# Bring up all app containers onto Docker
cd $BUILDPATH
docker-compose up -d
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: docker-compose fails.\n***********\n"
    exit $STATUS
fi

# Run tests
cd $WORKSPACE
bazel test //platform/vms:go_default_test
