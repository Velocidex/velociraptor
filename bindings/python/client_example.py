#!/usr/bin/python

"""Example Velociraptor api client.

This example demonstrates how to connect to the Velociraptor server
and issue a server side VQL query.

In this example we issue an event query which streams results slowly
to the api client. This demonstrates how to build reactive post
processing scripts as required.
"""
import argparse
import json
import grpc
import yaml

import api_pb2
import api_pb2_grpc

def run(config, query):
    # Fill in the SSL params from the api_client config file. You can get such a file:
    # velociraptor --config server.config.yaml config api_client > api_client.conf.yaml
    creds = grpc.ssl_channel_credentials(
        root_certificates=config["ca_certificate"].encode("utf8"),
        private_key=config["client_private_key"].encode("utf8"),
        certificate_chain=config["client_cert"].encode("utf8"))

    # This option is required to connect to the grpc server by IP - we
    # use self signed certs.
    options = (('grpc.ssl_target_name_override', "VelociraptorServer",),)

    # The first step is to open a gRPC channel to the server..
    with grpc.secure_channel(config["api_connection_string"],
                             creds, options) as channel:
        stub = api_pb2_grpc.APIStub(channel)

        # The request consists of one or more VQL queries. Note that
        # you can collect artifacts by simply naming them using the
        # "Artifact" plugin.
        request = api_pb2.VQLCollectorArgs(
            max_wait=1,
            Query=[api_pb2.VQLRequest(
                Name="Test",
                VQL="select * from info()",
            )])

        # This will block as responses are streamed from the
        # server. If the query is an event query we will block here
        # forever.
        for response in stub.Query(request):

            # Each response represents a set of rows. The columns are
            # listed in their own field to ensure column order is
            # preserved.
            print(response.Columns)

            # The actual payload is a list of dicts. Each dict has
            # column names as keys and arbitrary values.
            package = json.loads(response.Response)
            print (package)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Example Velociraptor client.')
    parser.add_argument('config', type=str,
                        help='Path to the api_client config. You can generate such '
                        'a file with "velociraptor config api_client"')
    parser.add_argument('query', type=str, help='The query to run.')

    args = parser.parse_args()

    config = yaml.load(open(args.config).read())
    run(config, args.query)
