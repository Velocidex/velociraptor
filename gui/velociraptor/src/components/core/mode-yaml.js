import { VqlHighlightRules } from './mode-vql.js';

export class YamlHighlightRules extends window.ace.acequire("ace/mode/yaml_highlight_rules").YamlHighlightRules {
    constructor() {
        super();

        this.$rules["start"] = [{
            token : "keyword",
            regex : /\s*-?\s*(name|type|description|choices|sources|parameters|author|reference|required_permissions|resources|tools|parameters|url|default|serve_locally|github_asset_regex|github_project|column_types|imports|export|notebook|template|timeout|ops_per_second|max_rows|max_upload_bytes):/,
        }, {
            token: "keyword",
            regex: /.*(precondition|query):\s*[|]?/,
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
