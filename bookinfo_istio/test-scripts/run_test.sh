hub="docker.io/kimikowang"

SCRIPTDIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
WORKSPACE=$SCRIPTDIR/../..
BINDIR=$WORKSPACE/bazel-bin
APPSDIR=$SCRIPTDIR/../apps
DISCOVERYDIR=$SCRIPTDIR/../discovery
BUILDPATH=$SCRIPTDIR/../docker
PILOTAGENTPATH=$WORKSPACE/cmd/pilot-agent
PILOTDISCOVERYPATH=$WORKSPACE/cmd/pilot-discovery

# Build the pilot agent binary
cd $PILOTAGENTPATH && bazel build :pilot-agent
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: build failed for pilot agent.\n***********\n"
    exit $STATUS
fi

# Build the pilot discovery binary
cd $PILOTDISCOVERYPATH && bazel build :pilot-discovery
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: build failed for pilot discovery.\n***********\n"
    exit $STATUS
fi

# Build the app binaries
set -x
set -o errexit

# Copy the pilot agent binary to each app dir
# Build the images and  push them to hub
for app in details productpage ratings reviews; do
  rm -f $APPSDIR/$app/pilot-agent && cp $BINDIR/cmd/pilot-agent/pilot-agent $_  
  make -C $APPSDIR/$app build build.dockerize docker.push clean GOOS=linux GOARCH=amd64 HUB=$hub
  rm -f $APPSDIR/$app/pilot-agent
done

# Build discovery image
cd $DISCOVERYDIR
rm -f pilot-discovery && cp $BINDIR/cmd/pilot-discovery/pilot-discovery $_  
docker build -t $hub/discovery:latest .
docker push $hub/discovery:latest
rm -f pilot-discovery

# Bring up all app containers
cd $BUILDPATH
docker-compose up -d
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: docker-compose fails.\n***********\n"
    exit $STATUS
fi

# Run tests
#cd $WORKSPACE
#bazel test //platform/vms:go_default_test
