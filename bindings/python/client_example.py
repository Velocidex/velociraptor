#!/usr/bin/python

"""Example Velociraptor api client.

This example demonstrates how to connect to the Velociraptor server
and issue a server side VQL query.

In this example we issue an event query which streams results slowly
to the api client. This demonstrates how to build reactive post
processing scripts as required.
"""

import json
import grpc

import api_pb2
import api_pb2_grpc

def run():
    # The first step is to open a gRPC channel to the server. By
    # default the server is listening over a unix domain socket.
    with grpc.insecure_channel('unix:///tmp/velociraptor_api.sock') as channel:
        stub = api_pb2_grpc.APIStub(channel)

        # The request consists of one or more VQL queries. Note that
        # you can collect artifacts by simply naming them using the
        # "Artifact" plugin.
        request = api_pb2.VQLCollectorArgs(
            max_wait=1,
            Query=[api_pb2.VQLRequest(
                Name="Test",
                VQL="select * from Artifact.Generic.Client.Stats()",
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
    run()
