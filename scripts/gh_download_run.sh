#!/bin/bash

# This script downloads the failed artifacts from the github actions
# for the last failed run.

i=$(gh api /repos/Velocidex/velociraptor/actions/runs?per_page=10  | jq 'limit(1; .workflow_runs[] | select (.name | contains("Windows")) | .id )')

gh run download $i
make UpdateCIArtifacts
