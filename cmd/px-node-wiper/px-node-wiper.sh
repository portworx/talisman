#!/bin/bash

LFILE_PREFIX=/var/cores/px-node-wipe

(
set -x

WAIT=0 # 0 means don't wait. Run to completion.
REMOVE_DATA=0 #0 means do not delete data
STATUS_FILE=/tmp/px-node-wipe-done
ETCPWX=/etc/pwx
OPTPWX=/opt/pwx
HOSTPROC1_NS=/hostproc/1/ns
PXCTLNM=pxctl
PXCTL=/opt/pwx/bin/${PXCTLNM}

rm -rf $STATUS_FILE

WFILES=$(ls -t ${LFILE_PREFIX}-*.log  2>/dev/null)
KEEP_NUM=3  # Keep latest <#> log files
CNT=0
for wfile in ${WFILES}; do  # clean up old logs
    [ -z "${wfile}" ] && continue
    ((CNT++))
    [ ${CNT} -lt ${KEEP_NUM} ] && continue
    echo "Removing old px-wipe log: ${wfile}"
    rm -f ${wfile}
done

DBUS_ADDR=/var/run/dbus/system_bus_socket
export DBUS_SESSION_BUS_ADDRESS="unix:path=${DBUS_ADDR}"

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

if [ -e ${DBUS_ADDR} ]; then
    /bin/systemctl --user stop portworx || true
    /bin/systemctl --user disable portworx || true
fi

rm -rf /etc/systemd/system/*portworx*
run_with_nsenter "systemctl daemon-reload" true

# the nsenter approach above doesn't seem to work on coreos machines. To cover all scenarios,
# try systemctl directly and ignore if it fails. This works on coreos.
systemctl daemon-reload
if [ -e ${DBUS_ADDR} ]; then
    /bin/systemctl --user daemon-reload || true
fi

# unmount oci
run_with_nsenter "umount $OPTPWX/oci" true

# pxctl node wipe
if [ ! -f "$PXCTL" ]; then
  echo "warning: path $PXCTL doesn't exist. Skipping $PXCTL sv node-wipe --all"
elif [ "$REMOVE_DATA" = "1" ]; then
  if [ ! -x "$PXCTL" ]; then
      # '/opt/pwx/bin/pxctl' exist however its not executable. Something is wrong so Check for container 'pxctl'
      nsenter --mount=/host_proc/1/ns/mnt -- sh -c '[ -x /opt/pwx/oci/rootfs/pwx_root/opt/pwx/bin/pxctl ]'
      if [ $? -eq 0 ]; then
	  # Container 'pxctl' exists, copy that to host to use for 'wipe'
	  nsenter --mount=/host_proc/1/ns/mnt -- cp /opt/pwx/oci/rootfs/pwx_root/opt/pwx/bin/pxctl $PXCTL
      fi
  fi
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

# Last check to see there are left over oci mounts
MOUNTS=$(nsenter --mount=$HOSTPROC1_NS/mnt -- mount)
if [ $? -eq 0 ]; then
    echo  "${MOUNTS}" |  while IFS= read -r line; do
	MPOINT=$(echo "${line}" | awk '{ print $3 }' | egrep 'pwx/oci');
	if [ $? -eq 0 ]; then
	    echo "Removing left over mount: ${MPOINT}"
	    nsenter --mount=$HOSTPROC1_NS/mnt -- umount ${MPOINT}
	fi
    done
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
) |& tee ${LFILE_PREFIX}-$(date +%s).log   # Save wipe output.
