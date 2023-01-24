FROM portworx/px-lib:bionic-gs-rel-2.13.0-base-gs-grpc-1.8.0

WORKDIR /

ADD px-node-wiper.sh /px-node-wiper.sh

ENTRYPOINT ["bash", "/px-node-wiper.sh"]
CMD []
