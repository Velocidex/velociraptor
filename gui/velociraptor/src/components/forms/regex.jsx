import React from 'react';
import PropTypes from 'prop-types';
import VeloAce from '../core/ace.jsx';
import Overlay from 'react-bootstrap/Overlay';
import Tooltip from 'react-bootstrap/Tooltip';
import "./regex.css";

import _ from 'lodash';

import T from '../i8n/i8n.jsx';

let gcompletions = ()=>[
    {name: "\\s",
     trigger: "\\",
     description: T("Match all spaces")},

    {name: "\\w",
     trigger: "\\",
     description: T("Match all words")},

    {name: "\\S",
     trigger: "\\",
     description: T("Match all non space")},

    {name: "\\\\",
     trigger: "\\",
     value: "\\\\\\\\",
     description: T("Plain backslash")},

    {name: "|",
     trigger: "|",
     description: T("Alternatives")},

    {name: "[]",
     trigger: "[",
     cursor_offset: 1,
     description: T("Character class (Matches any character inside the [])")},

    {name: "[^]",
     trigger: "[",
     cursor_offset: 2,
     description: T("Negated character class (Matches any character not in [])")},

    {name: "[0-9A-Z]",
     trigger: "[",
     description: T("Letters and numbers")},

    {name: "()",
     value: "()",
     trigger: "(",
     cursor_offset: 1,
     description: T("Capture groups")},

    {name: "(Alternate...|Alternate...)",
     value: "(|)",
     trigger: "(",
     cursor_offset: 1,
     description: T("Capture with alternates")},

    {name: "(?P<Name>...)",
     value: "(?P<>)",
     trigger: "(",
     cursor_offset: 4,
     description: T("Named capture group")},

    {name: "*?",
     trigger: "*",
     description: T("Zero or more matches, prefer fewer")},

    {name: "+?",
     trigger: "+",
     description: T("One or more matches, prefer fewer")},

    {name: "*",
     trigger: "*",
     description: T("Zero or more matches")},

    {name: "+",
     trigger: "+",
     description: T("One or more matches")},

    {name: "{min,max}",
     value: "{,}",
     cursor_offset: 1,
     trigger: "{",
     description: T("Match between min number and max number")},

];

let Completer = {
    // When the last part matches this, the completer kicks in. We
    // want it to triggr on ?
    identifierRegexps: [/\?|\\|\[|\(|\||\?|\*|\{/],

    getCompletions: (editor, session, pos, prefix, callback) => {
        let completions = [];

        _.each(gcompletions(), x=>{
            if (prefix === "?" || x.trigger === prefix) {
                let completion = {
                    caption: x.name,
                    description: T(x.description),
                    snippet: x.value || x.name,
                    type: x.description,
                    value: x.value || x.name,
                    score: 100,
                    docHTML: '<div class="arg-help">' + T(x.description) + "</div>",
                };

                if (prefix === "?") {
                    // Prefix the completion with ? so it always displays
                    completion.caption = "?" + completion.caption;
                }

                if (x.cursor_offset) {
                    completion.completer = {
                        insertMatch: function(editor, data) {
                            let pos = editor.selection.getCursor();
                            let text = editor.getValue();
                            let rows = text.split("\n");
                            let current_row = rows[pos.row];

                            // Strip the trigger from the match.
                            let new_row = current_row.substring(
                                0, pos.column - prefix.length) +
                                data.value +
                                current_row.substring(pos.column);
                            rows[pos.row] = new_row;

                            editor.setValue(rows.join("\n"));
                            editor.selection.moveTo(
                                pos.row, pos.column + x.cursor_offset - 1);
                        }
                    };
                }

                completions.push(completion);
            }
        });
        callback(null, completions);
    }
};


export default class RegEx extends React.Component {
    static propTypes = {
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        error: "",
    };

    constructor(props) {
        super(props);
        this.myRef = React.createRef();
    }

    aceConfig = (ace) => {
        ace.completers = [Completer];
        ace.setOptions({
            maxLines: 5,
            enableLiveAutocompletion: true,
            enableBasicAutocompletion: true,
            showGutter: false,
            placeholder: T("? for suggestions"),
        });
        this.setState({ace: ace});
    };

    setValue = value=>{
        let error = "";
        try {
            let sanitized_re = value.replace(/\(\?[^)]+\)/, "");
            // This raises an exception
            new RegExp(sanitized_re);
        } catch(e) {
            error = e.message;
        }
        if (this.state.error !== error) {
            this.setState({error: error});
        }
        this.props.setValue(value);
    }
    render() {
        return (
            <>
              <div ref={this.myRef}
                className="regex-form"
              >
                <Overlay target={this.myRef}
                         show={!_.isEmpty(this.state.error)}
                         placement="top">
                {(props) => (
                    <Tooltip className="regex-syntax-error" {...props}>
                      {this.state.error}
                    </Tooltip>
                )}
              </Overlay>
              <VeloAce text={this.props.value}
                       focus={false}
                       className="regex-form"
                       aceConfig={this.aceConfig}
                       onChange={this.setValue}
                       mode="regex" />
              </div>
            </>
        );
    }
};
