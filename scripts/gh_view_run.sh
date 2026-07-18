#!/bin/bash

# This script fetches the last failed run and views it.

base=$(basename `pwd`)

i=$(gh api "/repos/Velocidex/$base/actions/runs?per_page=1&status=failure" | jq .workflow_runs[0].id)

echo $i

gh run view $i --log-failed | less
