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

make -C $APPSDIR/productpage build GOOS=linux GOARCH=amd64
make -C $APPSDIR/details build GOOS=linux GOARCH=amd64
make -C $APPSDIR/ratings build GOOS=linux GOARCH=amd64
make -C $APPSDIR/reviews build GOOS=linux GOARCH=amd64

# Copy the a8registry binary to registry dir
#cp $BINDIR/a8registry $REGDIR/

# Copy the pilot binary to each app dir
for app in details productpage ratings reviews; do
  rm -rf $APPSDIR/$app/pilot && cp $BINDIR/cmd/pilot/pilot $_  
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
