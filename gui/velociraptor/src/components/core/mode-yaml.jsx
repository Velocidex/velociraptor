import { VqlHighlightRules } from './mode-vql.jsx';

// A list of artifact definition keywords - taken from artifact.proto
const keywords = [
    "name",
    "aliases",
    "description",
    "author",
    "reference",
    "references",
    "required_permissions",
    "implied_permissions",
    "impersonate",
    "resources",
    "tools",
    // Excluded because there is special handling "precondition",
    "parameters",
    "type",
    "sources",
    "imports",
    // Excluded because there is special handling "export",

    // deprecated reports
    "column_types",

    // ArtifactSource
    // Excluded because there is special handling "query",

    // deprecated  queries
    "notebook",

    // ArtifactParameter
    "default",
    "choices",
    "friendly_name",
    "validating_regex",
    "artifact_type",


    // Tools
    "url",
    "github_project",
    "github_asset_regex",
    "serve_locally",
    "expected_hash",
    "version",

    // Resources
    "timeout",
    "ops_per_second",
    "cpu_limit",
    "iops_limit",
    "max_rows",
    "max_upload_bytes",
    "max_batch_wait",
    "max_batch_rows",
    "max_batch_rows_buffer",
];


const KeywordRegexp = new RegExp("\\s*-?\\s*(" + keywords.join("|") + "):");

export class YamlHighlightRules extends window.ace.acequire("ace/mode/yaml_highlight_rules").YamlHighlightRules {
    constructor() {
        super();

        this.$rules["start"] = [{
            token : "keyword",
            regex : KeywordRegexp,
        }, {
            token: "keyword",
            regex: /.*(export|precondition|query):\s*[|]?/,
            onMatch: function(val, state, stack, line) {
                line = line.replace(/ #.*/, "");
                var indent = /^\s*((:\s*)?-(\s*[^|>])?)?/.exec(line)[0]
                    .replace(/\S\s*$/, "").length;
                var indentationIndicator = parseInt(/\d+[\s+-]*$/.exec(line));
                if (indentationIndicator) {
                    indent += indentationIndicator - 1;
                    this.next = "vql-start";
                } else {
                    this.next = "mlStringPreVql";
                }
                if (!stack.length) {
                    stack.push(this.next);
                    stack.push(indent);
                } else {
                    stack[0] = this.next;
                    stack[1] = indent;
                }
                return this.token;
            },
            next : "vql-start"
        }, {
            token: "invalid",
            regex: /(\w+?)(\s*:(?=\s|$))/
        }, {
            token : "invalid",
            regex: /^(\s*\w.*?)(:(?=\s|$))/
        }].concat(this.$rules["start"]);

        this.$rules["mlStringPreVql"] = [
            {
                token : "indent",
                regex : /^\s*$/
            }, {
                token : "indent",
                regex : /^\s*/,
                onMatch: function(val, state, stack) {
                    var curIndent = stack[1];

                    if (curIndent >= val.length) {
                        this.next = "start";
                        stack.splice(0);
                        return "invalid";
                    }
                    else {
                        stack[1] = val.length - 1;
                        this.next = stack[0] = "vql-start";
                    }
                    return this.token;
                },
                next : "vql-start"
            }
        ];

        this.embedRules(VqlHighlightRules, "vql-");
        this.$rules["vql-start"] = [
            // Empty lines do not affect the indents.
            {
                token : "indent",
                regex : "^\\s*$"
            }, {
                // If the indent is exactly the same as the previous
                // line, then render as vql, otherwise this is the
                // rest of the yaml file.
                token : "indent",
                regex : "^\\s*",
                onMatch: function(val, state, stack) {
                    var curIndent = stack[1];

                    if (curIndent >= val.length) {
                        this.next = "start";
                        stack.splice(0);
                    }
                    else {
                        this.next = "vql-start";
                    }
                    return this.token;
                },
            }].concat(this.$rules["vql-start"]);

        this.normalizeRules();
    }
}


export default class YamlMode extends window.ace.acequire('ace/mode/yaml').Mode {
    constructor() {
        super();

        this.HighlightRules = YamlHighlightRules;
    }
}
