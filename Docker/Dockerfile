FROM alpine:latest
LABEL description="Velociraptor docker server container"
LABEL maintainer="The Velociraptor Team: support@velocidex.com"
COPY ./entrypoint /bin/entrypoint
COPY ./init.vql /bin/init.vql
COPY ./bin/velociraptor /bin/velociraptor
COPY ./custom_artifacts /custom_artifacts/
WORKDIR /
CMD ["/bin/sh", "/bin/entrypoint"]
EXPOSE 8000
EXPOSE 8889
