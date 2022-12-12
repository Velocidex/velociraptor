import React from 'react';
import PropTypes from 'prop-types';
import VeloAce from '../core/ace.jsx';
import language_tools from 'ace-builds/src-min-noconflict/ext-language_tools.js';
import "./regex.css";
import T from '../i8n/i8n.jsx';

import _ from 'lodash';

let gcompletions=[
    {name: "rule template",
     trigger: "r",
     value: "rule Hit {\n   strings:\n     $a = \"keyword\" nocase wide ascii\n    condition:\n      any of them\n}\n",
     cursor_offset: 5,
     description: "Yara Rule Template"},
];

let Completer = {
    // When the last part matches this, the completer kicks in. We
    // want it to triggr on ?
    identifierRegexps: [/\?|\\|\[|\(|\||\?|\*|\{/],

    getCompletions: (editor, session, pos, prefix, callback) => {
        let completions = [];

        _.each(gcompletions, x=>{
            if (prefix === "?" || x.trigger === prefix) {
                let completion = {
                    caption: x.name,
                    description: T(x.description),
                    snippet: x.value || T(x.name),
                    type: T(x.description),
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


export default class YaraEditor extends React.Component {
    static propTypes = {
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        error: "",
    };

    aceConfig = (ace) => {
        ace.completers = [Completer, language_tools.textCompleter];
        ace.setOptions({
            enableLiveAutocompletion: true,
            enableBasicAutocompletion: true,
            autoScrollEditorIntoView: true,
            placeholder: T("? for suggestions"),
            showGutter: false,
            maxLines: 25,
        });
        this.setState({ace: ace});
    };

    setValue = value=>{
        // Todo: Verify the yara rule somehow.
        this.props.setValue(value);
    }

    render() {
        return (
            <>
              <div>
                <VeloAce text={this.props.value}
                         focus={false}
                         className="regex-form"
                         aceConfig={this.aceConfig}
                         onChange={this.setValue}
                         mode="yara" />
              </div>
            </>
        );
    }
};
