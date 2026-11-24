# Mirror the JSON name and omitempty option on generated protobuf fields.
/^[[:space:]]+[[:alnum:]_]+[[:space:]].*`protobuf:"/ {
    / yaml:"/!s/ json:("[^"]*")/ json:\1 yaml:\1/
}
