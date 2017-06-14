SCRIPTDIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
MAKEDIR=$SCRIPTDIR/../../../../../../../
BINDIR=$MAKEDIR/bin
REGDIR=$SCRIPTDIR/../registry
APPSDIR=$SCRIPTDIR/../apps
BUILDPATH=$SCRIPTDIR/../build/manifest.yml
SRCDIR=$SCRIPTDIR/../../

# Build the registry binary
make -C $MAKEDIR build.registry GOOS=linux GOARCH=amd64
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: make failed for registry.\n***********\n"
    exit $STATUS
fi

# Build the sidecar binary
make -C $MAKEDIR build.sidecar GOOS=linux GOARCH=amd64
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: make failed for sidecar.\n***********\n"
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
cp $BINDIR/a8registry $REGDIR/

# Copy the a8sidecar binary to each app dir
cp $BINDIR/a8sidecar $APPSDIR/details
cp $BINDIR/a8sidecar $APPSDIR/productpage
cp $BINDIR/a8sidecar $APPSDIR/ratings
cp $BINDIR/a8sidecar $APPSDIR/reviews

# Bring up all app containers onto CF
cf push -f $BUILDPATH 
STATUS=$?
if [ $STATUS -ne 0 ]; then
    echo -e "\n***********\nFAILED: cf push fails.\n***********\n"
    exit $STATUS
fi

# Run tests
cd $SRCDIR
go test -run ''
