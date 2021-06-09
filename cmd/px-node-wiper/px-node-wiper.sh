#!/bin/bash

WAIT=0 # 0 means don't wait. Run to completion.
REMOVE_DATA=0 #0 means do not delete data
STATUS_FILE=/tmp/px-node-wipe-done
ETCPWX=/etc/pwx
OPTPWX=/opt/pwx
HOSTPROC1_NS=/hostproc/1/ns
PXCTLNM=pxctl
PXCTL=/opt/pwx/bin/${PXCTLNM}

rm -rf $STATUS_FILE


fatal() {
    echo "" 2>&1
    echo "$@" 2>&1
    exit 1
}

usage() {
    echo "usage: $0 [-w <wait-on-success>] ] | [-h]]"
}

cleanup () {
  kill -s SIGTERM $!
  exit 0
}

sleep_forever() {
  while : ; do
    sleep 60 &
    wait $!
  done
}

# run_with_nsenter runs the given command in the host's proc/1 namespace using nsenter
# args: $0 command to run
#       $1 [true|false] if true, error is ignored
run_with_nsenter() {
  if [ "$#" -ne "2" ]; then
    fatal "insufficient number of arguments ($#) passed to run_with_nsenter()"
  fi

  echo "running $1 on the host's proc/1 namespace"
  nsenter --mount=$HOSTPROC1_NS/mnt -- $1
  if [ "$?" -ne "0" ]; then
    if [ "$2" = false ]; then
      fatal "nsenter command: $1 failed with exit code: $?"
    else
      echo "ignoring failure of nsenter command: $1"
    fi
  fi
}

trap cleanup SIGINT SIGTERM

while [ "$1" != "" ]; do
  case $1 in
    -w | --wait)       WAIT=1
                       ;;
    -r | --removedata) REMOVE_DATA=1
                       ;;
    -h | --help)       usage
                       exit
                       ;;
    * )                usage
                       exit 1
  esac
  shift
done

##### pre-checks
[ -d "$ETCPWX" ] || \
  fatal "error: path $ETCPWX doesn't exist. Ensure you started the container with $ETCPWX mounted"

[ -d "$HOSTPROC1_NS" ] || \
  fatal "error: path $HOSTPROC1_NS doesn't exist. Ensure you started the container with $HOSTPROC_NS mounted"


# Remove systemd service (if any)

run_with_nsenter "systemctl stop portworx" true
run_with_nsenter "systemctl disable portworx" true

# the nsenter approach above doesn't seem to work on coreos machines. To cover all scenarios,
# try systemctl directly and ignore if it fails. This works on coreos.
systemctl stop portworx || true
systemctl disable portworx || true

rm -rf /etc/systemd/system/*portworx*
run_with_nsenter "systemctl daemon-reload" true
systemctl daemon-reload

# unmount oci
run_with_nsenter "umount $OPTPWX/oci" true

# pxctl node wipe
if [ ! -f "$PXCTL" ]; then
  echo "warning: path $PXCTL doesn't exist. Skipping $PXCTL sv node-wipe --all"
elif [ "$REMOVE_DATA" = "1" ]; then
  # Nodewipe needs to be run on the host if poxxible cause multipath check issues.
  echo "Running pxctl nodewipe on the host's proc/1 namespace"
  nsenter --mount=$HOSTPROC1_NS/mnt -- "$PXCTLNM" sv node-wipe --all
  if [ "$?" -ne "0" ]; then
      "$PXCTL" sv node-wipe --all || \
	  fatal "error: node wipe failed with code: $?"
  fi
else
  echo "-removedata argument not specified. Not wiping the drives"
fi

# Remove binary files
run_with_nsenter "rm -fr $OPTPWX" false

if [ "$REMOVE_DATA" = "1" ]; then
  # Remove configuration files
  chattr -i /etc/pwx/.private.json || true
  chattr -i /etc/pwx/config.json || true
  rm -rf "$ETCPWX"/{.??,}*
  if [ $? -eq 0 ]; then
    touch "$STATUS_FILE"
    if [ "$WAIT" = "1" ]; then
      echo "Successfully wiped px. Sleeping..."
      sleep_forever
    fi
  else
      fatal "error: remove pwx configuration failed with code: $?"
  fi
else
  touch "$STATUS_FILE"
  echo "Successfully wiped px. Sleeping..."
  sleep_forever
fi
