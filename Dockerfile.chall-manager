FROM pulumi/pulumi-go:3.101.1
RUN pulumi login --local
COPY chall-manager /chall-manager
ENTRYPOINT [ "/chall-manager" ]
