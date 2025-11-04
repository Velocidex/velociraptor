

export class SigmaHighlightRules extends window.ace.acequire("ace/mode/yaml_highlight_rules").YamlHighlightRules {
    constructor() {
        super();

        this.$rules["start"] = [{
            token : "keyword",
            regex : /\s*-?\s*(title|logsource|product|category|service|description|detection|condition|selection[a-z]+|status|author|level|references|date|modified|details|id):/,
        }, {
            token: "invalid",
            regex: /(\w+?)(\s*:(?=\s|$))/
        }, {
            token : "invalid",
            regex: /^(\s*\w.*?)(:(?=\s|$))/
        }].concat(this.$rules["start"]);
        this.normalizeRules();
    }
}


export default class SigmaMode extends window.ace.acequire('ace/mode/yaml').Mode {
    constructor() {
        super();

        this.HighlightRules = SigmaHighlightRules;
    }

    static path = "ace/mode/sigma";
}
