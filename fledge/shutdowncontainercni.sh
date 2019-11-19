#! /bin/sh

if [[ -z "${2:-}" ]]; then
  echo "Use: shutdowncontainercni.sh netns cniif" 1>&2
  exit 1
fi

netns=${1}
shift
cniif=${1}
shift

ip netns exec $netns ip link delete $cniif
rm /var/run/netns/$netns

