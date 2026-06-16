"""This is a helper script for debugging Velociraptor.

You can load it into your dlv session with:

```
$ make debug
dlv debug --wd=. --build-flags="-tags 'server_vql extras'" ./bin/ -- frontend --disable-panic-guard -v --debug
Type 'help' for list of commands.
(dlv) source scripts/dlv.star
Loading Velociraptor extenstions
(dlv)
```

This debugger extension has the following benefits:
- pp command pretty prints structs:
   1. special support for Velociraptor types like ordereddict.Dict, Scope etc.
   2. Hide unexported fields by default (use ppl to see a longer dump).
   3. Support protobuf by hiding internal fields.
   4. Coloring output to make it easier to see

- Also some helpful commands

"""

# Set up the color theme
reset_code = "\033[0m"
list_line_color = "\x1b[34m"
keyword_color = "\x1b[94m"
string_color = "\x1b[91m"
number_color = "\x1b[37m"
comment_color = "\x1b[92m"
arrow_color = "\x1b[93m"
tab_color = "\x1b[90m"

proto_fields = ["state", "sizeCache", "unknownFields"]

LongOptions = {
    "include_private": True,
    "include_defaults": True,
    "shorten_strings": False,
}

ShortOptions = {
    "include_private": False,
    "include_defaults": False,
    "shorten_strings": True,
}

def set_config():
    dlv_command("config source-list-line-color " + list_line_color)
    dlv_command("config source-list-keyword-color " + keyword_color)
    dlv_command("config source-list-string-color " + string_color)
    dlv_command("config source-list-number-color " + number_color)
    dlv_command("config source-list-comment-color " + comment_color)
    dlv_command("config source-list-arrow-color " + arrow_color)
    dlv_command("config source-list-tab-color " + tab_color)
    dlv_command("config max-variable-recurse 5")

def debug_Variable(x):
    print(repr(x))
    for k in dir(x):
        if k == "Value":
            continue
        print("Variable", k, repr(getattr(x, k)))

def get_lines(loc, before, after):
    return "\n".join(read_file(loc.File).splitlines()[loc.Line+before:loc.Line+after])

def get_var(x, name):
    for c in x.Children:
        if c.Name == name:
            return c
    return None

def command_gsl(filter):
    """Iterate over all goroutines and print the current line they are at"""
    result = ""
    gs = goroutines().Goroutines
    gs = sorted(gs, key= lambda x: x.ID)

    for g in gs:
        loc = g.UserCurrentLoc
        cur_line = get_lines(loc, 0, 1)
        if not filter in cur_line:
            continue

        line = get_lines(loc, -4, -1) + "\n------->" + get_lines(loc, 0, 4)
        result += "%d:\t%s:%d\t%s\n" % (g.ID, loc.File, loc.Line, line)
    print(result)

def strip_prefix(value, prefix):
    if value.startswith(prefix):
        return value[len(prefix):]
    return value

def format(value):
    return strip_prefix(repr(value), "interface {}")

def format_dict(x, indent="", options=LongOptions):
    if x.Kind == "interface" and x.Children:
        x = x.Children[0]

    if x.Kind == "ptr" and x.Children:
      x = x.Children[0]

    items = get_var(x, "items")
    if not items:
        return indent + "%s<Unmapped>%s\n" % (comment_color, reset_code)

    result = ""
    for idx, item in enumerate(items.Children):
        key = item.Value.Key
        value = get_var(item, "Value")

        result += indent + "  %s%d%s %s%s%s: %s\n" % (
            number_color, idx, reset_code,
            keyword_color, key, reset_code,
            format_type(value, options=options, indent=indent + "    "))
    return result

def format_type(x, indent="", options=LongOptions):
    if x.Kind == "interface" and x.Children:
        x = x.Children[0]

    if x.Kind == "ptr" and x.Children:
      x = x.Children[0]

    if x.Kind == "chan" and x.Children:
        closed = get_var(x, "closed")
        res = "%s%s%s" % (comment_color, x.Type, reset_code)
        if closed.Value:
            res += " (closed) "
        return res

    if x.Kind == "string":
        res =  str(x.Value)
        if options["shorten_strings"]:
            lines = res.split("\n")
            if len(lines) > 1:
                res = lines[0] + "%s ...%s" % (tab_color, reset_code)

            if len(res) > 100:
                res = res[:100] + "%s ...%s" % (tab_color, reset_code)
        return res

    if x.Unreadable:
        return "%s<unreadable>%s" % (comment_color, reset_code)

    if x.Len > 0 and len(x.Children) == 0:
        return "%s<unloaded>%s" % (comment_color, reset_code)

    var = x.Value

    if x.Type == "time.Time":
        return str(var).split("(")[1].split(")")[0]

    if x.Type == "void":
        return "%snil%s" % (comment_color, reset_code)

    if len(x.Children) == 0:
        return str(var)

    if x.Type == "github.com/Velocidex/ordereddict.Dict":
        result = indent + "%sDict%s\n" % (string_color, reset_code)
        result += format_dict(x, indent=indent, options=options)
        return result.rstrip()

    if x.Type == "www.velocidex.com/golang/vfilter/scope.Scope":
        result = indent + "Scope (%d)\n" % var.id
        vars = get_var(x, "vars")
        for idx, v in enumerate(vars.Children):
            result += indent + "%sVar %s%s\n" % (string_color, idx, reset_code)
            result += format_dict(v, indent=indent + "  ",
                                  options=options)

        return result.rstrip()

    if x.Type == "regexp.Regexp":
        result = indent + "%sregexp.Regexp%s: %s" % (
            comment_color, reset_code, repr(var.expr))
        return result

    if x.Kind == "slice":
        result = "%s%s%s len %d\n" % (
            string_color, x.Type, reset_code, len(x.Children))

        for idx, v in enumerate(x.Children):
            result += indent + "%s%s%s %s\n" % (
                number_color, idx, reset_code,
                format_type(v, indent=indent+"  ", options=options))

        return result.rstrip()

    if x.Kind == "struct":
        # Format protobufs specially by dropping useless fields.
        is_proto = False
        omitted_fields = False
        result = " %s%s%s {\n" % (string_color, x.Type, reset_code)
        for c in x.Children:
            if not options["include_defaults"] and is_default(c):
                omitted_fields = True
                continue

            if not options["include_private"] and is_private(c):
                omitted_fields = True
                continue

            if c.Name == "state":
                is_proto = True

            if is_proto and (c.Name in proto_fields or is_default(c)):
                omitted_fields = True
                continue

            result += indent + "  %s%s%s: %s\n" % (
                keyword_color, c.Name, reset_code,
                format_type(c, indent=indent + "  ", options=options))

        if omitted_fields:
            result += indent + "%s  ... omitted_fields%s\n" % (
                tab_color, reset_code)

        return result.rstrip()+ "\n" + indent + "}"

    if x.Kind == "map":
        result = "%s%s%s {\n" % (string_color, x.Type, reset_code)
        k = ""
        for idx, c in enumerate(x.Children):
            if idx % 2 == 0:
                k = format(c.Value)
                continue

            result += indent + "  %s%s%s: %s\n" % (
                keyword_color, k, reset_code,
                format_type(c, indent=indent + "  "))
        return result.rstrip()+ "\n" + indent + "}"

    return indent + str(var)

def is_private(x):
    return x.Name[0] == x.Name[0].lower()

def is_default(x):
    if (x.Kind == "ptr" or x.Kind == "interface") and len(x.Children) > 0:
        x = x.Children[0]

    if x.Type == "void":
        return True

    if x.Type == "string" and x.Value == "":
        return True

    if (x.Kind == "struct" or x.Kind == "slice") and len(x.Children) == 0:
        return True

    if (x.Kind == "uint" or x.Kind == "int") and x.Value == 0:
        return True

    if (x.Kind == "float32" or x.Kind == "float64") and x.Value == 0.0:
        return True

    if x.Kind == "bool" and not x.Value:
        return True

    return False

def command_ppl(args):
    """Pretty print local variables.

This supports some internal Velociraptor types:
* ordereddict.Dict
* vfilter.Scope
* Protocol buffers

better display for structs

Example:
> ppl item
    """
    result = ""
    x = eval(None, args).Variable
    print(format_type(x))

def command_pp(args):
    """Concise Pretty print local variables.

This supports some internal Velociraptor types:
* ordereddict.Dict
* vfilter.Scope
* Protocol buffers
* regex.Regexp

better display for structs

Example:
> pp item
    """
    result = ""
    x = eval(None, args).Variable
    print(format_type(x, options=ShortOptions))


def command_btt(context=0):
    """Print decorated backtrace (enriched bt).

This adds a listing of each call site in the backtrace.
    """
    cur_goroutine = state().State.SelectedGoroutine
    st = stacktrace(cur_goroutine.ID, 5)
    for idx, f in enumerate(st.Locations):
       if "go/src/runtime" in f.File:
          continue

       print("%d %s%s%s\n\t\t\t%s%s%s %s%d%s" %( idx,
             comment_color, f.Function.Name_, reset_code,
             string_color, f.File, reset_code,
             number_color, f.Line, reset_code))
       if context > 0:
              print(keyword_color +
                 get_lines(f, -context,-1) + "\n*" + get_lines(f, -1, context) +
                 reset_code)

def command_rb(args):
    """Rebuild and restart"""
    print("Rebuilding")
    dlv_command("rebuild")
    dlv_command("restart")
    dlv_command("continue")

def main():
    set_config()
    print("Loading Velociraptor extenstions")
