

export class MarkdownHighlightRules extends window.ace.acequire("ace/mode/markdown_highlight_rules").MarkdownHighlightRules {
    constructor() {
        super();

        this.$rules.start.unshift({
            token: "string.other",
            regex: "\\{\\{",
            next: "template",
        });

        var keywords = (
            "query|table|LineChart|BarChart|TimeChart|ScatterChart"
        );

        var keywordMapper = this.createKeywordMapper({
            "keyword": keywords,
        }, "identifier", true);

        this.$rules["template"] = [{
            token: "string.other",
            regex: "\\}\\}",
            next: "start",
        }, {
            token : "string",           // " string
            regex : '".*?"'
        }, {
            token : "string",           // ' string
            regex : "'.*?'"
        }, {
            token : keywordMapper,
            regex : "[a-zA-Z_$][a-zA-Z0-9_$]*\\b"
        }, {
            token: "text",
            regex: ".+?",
        }];

        this.normalizeRules();
    }
}


export default class MarkdownMode extends window.ace.acequire('ace/mode/markdown').Mode {
    constructor() {
        super();

        this.HighlightRules = MarkdownHighlightRules;
    }

    static path = "ace/mode/md";
}
