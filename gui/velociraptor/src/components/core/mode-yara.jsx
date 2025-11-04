
// VQL mode is based on SQL mode but adds some more rules.
export class YaraHighlightRules extends window.ace.acequire("ace/mode/text_highlight_rules").TextHighlightRules {
    constructor() {
        super();

        this.$rules = {
            "start" : [
                {
                    token: "keyword",
                    regex: "rule",
                    next: "rule_name",
                },
            ],
            "rule_name": [
                {
                    token: "entity.name",
                    regex: "[a-zA-Z0-9_]+",
                },
                {
                    token: "paren.lparen",
                    regex: "[{]",
                    next: "rule_body",
                }
            ],
            "rule_body": [
                {
                    token: "paren.rparen",
                    regex: "[}]",
                    next: "start",
                },
                {
                    token: "keyword",
                    regex: "strings:",
                    next: "strings",
                },
                {
                    token: "keyword",
                    regex: "condition:",
                    next: "conditions",
                },
                {
                    token: "keyword",
                    regex: "meta:",
                    next: "meta",
                },
                {
                    defaultToken : "text",
                }
            ],
            "meta": [
                {

                },
                {
                    token: "keyword",
                    regex: "strings:",
                    next: "strings",
                },
                {
                    token: "keyword",
                    regex: "condition:",
                    next: "conditions",
                },
                {
                    defaultToken : "comment",
                }

            ],
            "strings": [
                {
                    token: "variable.language",
                    regex: "\\$[a-z0-9-_]+ =",
                },
                {
                    token: "string", // single line
                    regex : '["](?:(?:\\\\.)|(?:[^"\\\\]))*?["]',
                },
                {
                    token: "keyword",
                    regex: "wide|nocase|ascii",
                },
                {
                    // Hex lines
                    token: "string.language",
                    regex: "[{].+?[}]",
                },
                {
                    // Regex
                    token: "string.language",
                    regex: "/.+?/[sgim]?",
                },
                {
                    token: "keyword",
                    regex: "condition:",
                    next: "conditions",
                },
                {
                    token: "paren.rparen",
                    regex: "[}]",
                    next: "start",
                },
            ],
            "conditions": [
                {
                    token: "variable.language",
                    regex: "\\$[a-z0-9-_]+",
                },
                {
                    token: "paren.rparen",
                    regex: "[}]",
                    next: "start",
                },
            ],
        };

        this.normalizeRules();
    }
}


export default class YaraMode extends window.ace.acequire('ace/mode/text').Mode {
    constructor() {
        super();

        this.HighlightRules = YaraHighlightRules;
    }

    static path = "ace/mode/yara";
}
