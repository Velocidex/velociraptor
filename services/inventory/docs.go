package inventory

/*

## Velociraptor's Inventory service

The inventory service is responsible for managing external tools
within Velociraptor artifacts.

The inventory keeps metadata about the various third party tools known
to Velociraptor. Tools are stored internally by artifacts_proto.Tool:

1. A tool is known by `Name` and `Version`
2. A tool may be served by Velociraptor itself, or by an external URL (marked by `serve_url`).

### Defining tools

Tools can be defined within an artifact or by using the
inventory.AddTool() API. Note that defining the artifact does not
actually download the tool - tools are only downloaded when they are
"materialized". The artifact launcher materializes tools as needed
when an artifact is scheduled.

Tools are defined as part of the artifact definition. This means that
when the artifact is collected, the tool is materialized and made
available for download, then the relevant tool hashes and URLs are
injected into the query environment by the artifact compiler.

Logically the tool lifecycle is:

1. Tool defined in artifact definition

2. Tool definition is added to the inventory service (when artifact is
   compiled)

3. Artifact is launched -> Tool materialized: If a url is defined, the
   server will attempt to download the tool so the hash can be
   calculated.

4. The artifact is compiled: VQL Environment variables containing tool
   information are injected into the VQL stream by the VQL compiler.

5. Compiled artifact is scheduled for clients.

6. On the client, the artifact may call Generic.Utils.FetchBinary to
   fetch the tool and check its hash by pulling the URL and tool hash
   from the VQL environment.


### Tool versioning

Since each artifact may declare its own tool definition, it is common
for multiple artifacts to define the same tool. We therefore need some
logic to disambiguate conflicts in the tool definition.

Commonly different artifacts using the same tool might be written for
different version of that tool. For example there may be a difference
in command line args between versions.

Tool definitions may contain a version string. This should be a
semantic version that identifies the tool version. The combination of
tool name + tool version uniquely identifies a tool. In other words,
Velociraptor treats the combination of Tool Name and Tool Version as
the indentifier for the tool.

It is possible for multiple versions to co-exist within the
Inventory. Support the tool "X" has two versions, v1 and v2 and it is
defined in two separate artifacts:

name: Artifact1
tools:
- name: X
  version: v1


name: Artifact2
tools:
- name: X
  version: v2

Velociraptor ensures that when Artifact1 is launched, it receives the
X tool with v1.

It is generally recommended that all artifacts define a version for
their tool definitions. However if a version is not specified, the
artifact will be compiled with the highest tool version available
(according to Semantic Versioning comparison).


name: ArtifactHighest
tools:
- name: X

The above artifact will receive tool v2 when it is compiled.

*/
