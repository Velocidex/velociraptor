import _ from 'lodash';

let gcompletions = [];

// VQL mode is based on SQL mode but adds some more rules.
export class VqlHighlightRules extends window.ace.acequire("ace/mode/sql_highlight_rules").SqlHighlightRules {
    constructor() {
        super();

        var keywords = (
            "explain|select|from|where|and|or|group|by|order|limit|as|null|let"
        );

        var builtinConstants = (
            "true|false"
        );

        var builtinFunctions = "if";

        _.each(gcompletions, item=>{
            if (item.type === "Function" || item.type === "Plugin") {
                builtinFunctions += "|" + item.name;
            }
        });

        var keywordMapper = this.createKeywordMapper({
            "keyword": keywords,
            "support.function": builtinFunctions,
            "constant.language": builtinConstants,
        }, "identifier", true);

        this.$rules = {
            "start" : [ {
                token : "comment",
                regex : "--.*$"
            },  {
                token : "comment",
                regex : "//.*$"
            },  {
                token : "comment",
                start : "/\\*",
                end : "\\*/"
            }, {
                token : "string",           // " string
                regex : '".*?"'
            }, {
                token : "string.triple",           // ''' string
                regex: "'''",
                next: "mlString",
            }, {
                token : "string",           // ' string
                regex : "'.*?'"
            }, {
                token : "string",           // ` string (apache drill)
                regex : "`.*?`"
            }, {
                token : "constant.numeric", // float
                regex : "[+-]?\\d+(?:(?:\\.\\d*)?(?:[eE][+-]?\\d+)?)?\\b"
            }, {
                token : keywordMapper,
                regex : "[a-zA-Z_$][a-zA-Z0-9_$]*\\b"
            }, {
                token : "keyword.operator",
                regex : "\\+|\\*|\\-|\\/|\\/\\/|%|<@>|@>|<@|&|\\^|~|<|>|<=|=>|==|!=|<>|="
            }, {
                token : "paren.lparen",
                regex : "[\\(]"
            }, {
                token : "paren.rparen",
                regex : "[\\)]"
            }, {
                token : "text",
                regex : "\\s+"
            }],
            "mlString": [{
                token: "string.triple",
                regex: "'''",
                next: "start",
            }, {
                token: "string.triple",
                regex: /.+(?=''')/,
            }, {
                token: "string.triple",
                regex: /.+/,
            }],
        };
        this.normalizeRules();
    }
}


export default class VqlMode extends window.ace.acequire('ace/mode/sql').Mode {
    constructor() {
        super();

        this.HighlightRules = VqlHighlightRules;
    }

    static setCompletions = (completions) => {
        gcompletions = completions;
    }

    static path = "ace/mode/vql";
}
