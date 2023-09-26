FROM portworx/px-lib:bionic-hk-rel-3.0.1-base-grpc-1.8.0-lttng-2.12-go-1.20.7

WORKDIR /

ADD px-node-wiper.sh /px-node-wiper.sh

ENTRYPOINT ["bash", "/px-node-wiper.sh"]
CMD []
