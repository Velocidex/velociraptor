
// VQL mode is based on SQL mode but adds some more rules.
export class RegexHighlightRules extends window.ace.acequire("ace/mode/text_highlight_rules").TextHighlightRules {
    constructor() {
        super();

        this.$rules = {
            "start" : [
                {
                    token: "regexp.keyword.operator",
                    regex: "\\\\(?:u[\\da-fA-F]{4}|x[\\da-fA-F]{2}|.)"
                }, {
                    token : "invalid",
                    regex: /\{\d+\b,?\d*\}[+*]|[+*$^?][+*]|[$^][?]|\?{3,}/
                }, {
                    token : "constant.language.escape",
                    regex: /\(\?[:=!]|\)|\{\d+\b,?\d*\}|[+*]\?|[()$^+*?.]/
                }, {
                    token : "constant.language.delimiter",
                    regex: /\|/
                }, {
                    token: "constant.language.escape",
                    regex: /\[\^?/,
                    next: "regex_character_class"
                }, {
                    defaultToken: "text"
                }
            ],
            "regex_character_class": [
                {
                    token: "regexp.charclass.keyword.operator",
                    regex: "\\\\(?:u[\\da-fA-F]{4}|x[\\da-fA-F]{2}|.)"
                }, {
                    token: "constant.language.escape",
                    regex: "]",
                    next: "start"
                }, {
                    token: "constant.language.escape",
                    regex: "-"
                }, {
                    defaultToken: "string.regexp.charachterclass"
                }
            ],
        };

        this.normalizeRules();
    }
}


export default class RegexMode extends window.ace.acequire('ace/mode/text').Mode {
    constructor() {
        super();

        this.HighlightRules = RegexHighlightRules;
    }

    static path = "ace/mode/regex";
}
