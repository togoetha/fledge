#!/bin/bash

if [[ -z "${5:-}" ]]; then
  echo "Use: wdsetup_worker.sh arch runtime usevpn(true/false) interface vkubeURL ignoreKubeProxy" 1>&2
  exit 1
fi

arch=${1}
shift
runtime=${1}
shift
usevpn=${1}
shift
interface=${1}
shift
vkubeurl=${1}
shift
ignorekproxy=${1}

if [ "$arch" = "arm" ]; then
  if [ "$runtime" = "containerd" ]; then
    bash -x wdsetup_worker_ctd_arm.sh $usevpn $interface $vkubeurl $ignorekproxy
  else 
    bash -x wdsetup_worker_dock_arm.sh $usevpn $interface $vkubeurl $ignorekproxy
  fi
else 
  if [ "$runtime" = "containerd" ]; then
    bash -x wdsetup_worker_ctd.sh $usevpn $interface $vkubeurl $ignorekproxy
  else 
    bash -x wdsetup_worker_dock.sh $usevpn $interface $vkubeurl $ignorekproxy
  fi
fi
