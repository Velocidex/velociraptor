## Velociraptor Golden Tests

The files in this directory are the golden test suite used by the CI
pipeline.

What are Golden tests? Golden testing is a methodology to quickly and
efficiently write tests:

1. First a test case is written with the VQL queries that should be
   run. These queries are written in a file with a `.in.yaml`
   extension.
2. The `golden` test runner can be run on the test files using `make
   golden` at the top level of this repository.
3. If the output of the queries is different from the existing output
   (stored in `.out.yaml` files) the test will fail. The golden runner
   will then update the output file with the new data.
4. The user can compare the changes in the output file (e.g. using
   `git diff`) and if the changes are OK then simply `git add` the new
   output file. Running the golden tests again should produce no
   change.

By default the makefile rule runs the debug race detector binary (you
can built this using just `make` at the top level. This will produce a
debug build in `./output/velociraptor`. This binary includes the race
detector and so it is quite slow to run but worth it for tests.

If you find you need to iterate quicker you can manually run the
production binary (built using `make linux`) by modifying the command
run by the `make golden` command.

Additionally you can run the `dlv` debugger in the golden output by
running `make debug_golden` at the top level.

To filter the test cases (so they dont have to all run) you can set
the `GOLDEN` environment variable. For example to only run the tests
in `pe.in.yaml`:

```
$ GOLDEN=pe make golden
./output/velociraptor -v --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --filter=pe
```


## NOTES

Golden Testing requires the output to not change between subsequent
runs and when running between different environment. This means that
output that naturally changes should be avoided - for example output
that depends on:

- Time
- File paths
- Operating systems

You can use a combination of mocking plugin output and selecting
specific columns to format the output in such a way that it does not
depends on ephemeral things.


## Developing artifacts

When developing artifacts using TDD it is useful to load the raw
artifact YAML without needing to build the binary each time. This way
we can iterate over the artifact yaml and see the results immediately
in the golden out yaml.

An example command line is:

```
./output/velociraptor-v0.7.0-linux-amd64 -v --config artifacts/testdata/windows/test.config.yaml golden artifacts/testdata/server/testcases/ --env srcDir=`pwd` --filter=hostsfile --definitions artifacts/definitions/Generic/System/
```

Here the binary will force load the raw yaml definition at runtime
overriding the built in artifact definition. It will then run the
Golden test `hostsfile.in.yaml`
