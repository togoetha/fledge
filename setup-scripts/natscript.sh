#!/bin/bash

#
# This script enabled IPv4 NAT on wall1 and wall2, physical and virtual machines.
# It also persist this change on ubuntu/debian, so that it survives a machine reboot.
#
# This will off course not work on any distro. It has been tested on the default ubuntu/debian images.
# Patches to make it work on other images are always welcome.
#
# This script returns output that can be used by ansible.
#

SETUP_IGENT_VPN=1

if [[ $EUID -ne 0 ]]
then
    echo "{ \"failed\": true, \"msg\": \"This script should be run using sudo or as the root user\" }"
    exit 1
fi

GW=$(route -n | grep '^0.0.0.0' | grep UG | awk '{print $2}')

WALL=$(hostname | sed -e 's/.*\(wall[1-2]\).ilabt.iminds.be$/\1/')

change_default_gateway() {
    new_gw=$1

    #Add now
    route del default gw ${GW} && route add default gw ${new_gw}

    #Add on reboot
    if [[ -e /etc/network/interfaces ]]
    then
        sed -i -e"/^iface cnet inet manual$/a \ \ \ \ pre-up route del default gw ${GW}"\
               -e"/^iface cnet inet manual$/a \ \ \ \ post-up route add default gw ${new_gw}"\
                /etc/network/interfaces
    fi
}

add_route() {
    target=$1
    netmask=$2
    route_gw=$3

    #Add now
    route add -net ${target} netmask ${netmask} gw ${route_gw}

    #Add on reboot
    if [[ -e /etc/network/interfaces ]]
    then
        sed -i -e"/^iface cnet inet manual$/a \ \ \ \ post-up route add -net ${target} netmask ${netmask} gw ${route_gw}" /etc/network/interfaces
    fi
}


if [[ "$GW" = "10.2.15.253" ]] #Physical wall1-machines with IPv4 already set
then
    echo "{ \"changed\": false, \"msg\": \"Already set up IPv4 NAT for wall1 physical node\" }"
    exit 0
elif [[ "$GW" = "10.2.47.253" ]] #Physical wall2-machines with IPv4 already set
then
    echo "{ \"changed\": false, \"msg\": \"Already set up IPv4 NAT for wall2 physical node\" }"
    exit 0
elif [[ "$GW" = "172.16.0.2" ]] #Virtual machines with IPv4 already set
then
    echo "{ \"changed\": false, \"msg\": \"Already set up IPv4 NAT for wall virtual machine\" }"
    exit 0
elif [[ "$GW" = "10.2.15.254" ]] #Physical wall1-machines
then
    change_default_gateway '10.2.15.253'
    add_route '10.11.0.0' '255.255.0.0' $GW
    add_route '10.2.32.0' '255.255.240.0' $GW
    echo "{ \"changed\": true, \"msg\": \"Succesfully setup IPv4 NAT for wall1 physical node\" }"
elif [[ "$GW" = "172.16.0.1" && "${WALL}" = "wall1" ]] #Virtual wall1-machines
then
    add_route '10.2.0.0' '255.255.240.0' $GW
    change_default_gateway '172.16.0.2'
    echo "{ \"changed\": true, \"msg\": \"Succesfully setup IPv4 NAT for wall1 virtual machine\" }"
elif [[ "$GW" = "10.2.47.254" ]] #Physical wall2-machines
then
    change_default_gateway '10.2.47.253'
    add_route '10.11.0.0' '255.255.0.0' $GW
    add_route '10.2.0.0' '255.255.240.0' $GW
    echo "{ \"changed\": true, \"msg\": \"Succesfully setup IPv4 NAT for wall2 physical node\" }"
elif [[ "$GW" = "172.16.0.1" && "${WALL}" = "wall2" ]] #Virtual wall2-machines
then
    add_route '10.2.32.0' '255.255.240.0' $GW
    change_default_gateway '172.16.0.2'
    echo "{ \"changed\": true, \"msg\": \"Succesfully setup IPv4 NAT for wall2 virtual machine\" }"
else
    echo "{ \"failed\": true, \"changed\": false, \"msg\": \"Failed to detect testbed with GW=${GW}\" }"
    exit 1
fi

if [[ "$SETUP_IGENT_VPN" ]]
then
    add_route '157.193.214.0' '255.255.255.0' $GW
    add_route '157.193.215.0' '255.255.255.0' $GW
    add_route '157.193.135.0' '255.255.255.0' $GW
    add_route '192.168.126.0' '255.255.255.0' $GW
fi
