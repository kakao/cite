#!/usr/bin/env bash
APP_NAME=cite
IMAGE_NAME=docker-reg.c.9rum.cc/cite-core/${APP_NAME}
TAG=$(date +%Y%m%d-%H%M%S)
INSTANCE=4

## fleet server config
export FLEETCTL_ENDPOINT=http://cite-work.s2.krane.9rum.cc:49153
export FLEETCTL_ETCD_KEY_PREFIX=/fleet
export FLEETCTL_SSH_USERNAME=deploy

set -e

## windows subsystem for linux patch
if grep -q -i microsoft /proc/sys/kernel/osrelease; then
    IS_WSL=true
fi

function execute {
    CMD=$1
    if $IS_WSL; then
        CMD=${CMD/docker/wcmd docker}
    fi
    echo "command: $CMD"
    command $CMD
}

# echo "### vendoring dependencies"
# execute godep save

echo "### build"
# SRC_PATH="$PWD"
# if $IS_WSL; then
#     # alias docker='wcmd docker'
#     SRC_PATH=${SRC_PATH//\/mnt\/c/c:}
# fi
# execute "docker run --rm -v "$SRC_PATH":/go/src/github.daumkakao.com/ctf/${APP_NAME} \
#     -w /go/src/github.daumkakao.com/ctf/${APP_NAME} \
#     golang go build -v"
execute "docker build -t ${IMAGE_NAME}:${TAG} ."
execute "docker tag ${IMAGE_NAME}:${TAG} ${IMAGE_NAME}:latest"

echo "### docker push ${IMAGE_NAME}:${TAG}"
execute "docker push ${IMAGE_NAME}:${TAG}"
execute "docker push ${IMAGE_NAME}:latest"

OLD_UNITS=$(fleetctl list-units --fields=unit | grep ${APP_NAME}@ | tr '\n' ' ')
echo "-- deploy fleet unit --"
UNIT_NAMES=""
for i in $(seq 1 ${INSTANCE}); do
    UNIT_NAMES="$UNIT_NAMES ${APP_NAME}@${TAG}_${i}.service"
done
execute "fleetctl start $UNIT_NAMES"

echo "-- wait for startup --"
for i in $(seq 1 60); do
    UNIT_COUNT=$(fleetctl list-units | grep ${APP_NAME}@ | grep -v active | wc -l)
    echo activating : $UNIT_COUNT
    if [ $UNIT_COUNT == "0" ]; then
        echo "-- destroy old fleet units"
        execute "fleetctl destroy $OLD_UNITS"
        break
    fi
    sleep 1
done
