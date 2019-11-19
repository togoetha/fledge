Welcome to the FLEDGE repository!

Repository structure:

- clvers: contains a command line tool that allows FLEDGE to detect OpenCL version
- fledge: contains an older FLEDGE version as presented during SC2 2019
- fledge-integrated: contains the newest version of FLEDGE with an integrated virtual kubelet, GPU resource detection and a number of optimizations
- openvpn-client: FLEDGE currently uses OpenVPN for communication, this Dockerfile builds a container that can be used as a VPN client by FLEDGE
- setup-scripts: a number of helper scripts developed to set up a FLEDGE-compatible K8S node or a FLEDGE node for several hardware configurations. Warning: these are heavily tailored for the specific hardware/software environments used during the tests, but can be instructive.

For more information, contact togoetha.goethals@ugent.be
