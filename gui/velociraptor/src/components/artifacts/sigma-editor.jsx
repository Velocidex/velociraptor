import _ from 'lodash';

import PropTypes from 'prop-types';

import React, { Component } from 'react';
import Button from 'react-bootstrap/Button';
import Card from 'react-bootstrap/Card';
import T from '../i8n/i8n.jsx';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import { JSONparse } from '../utils/json_parse.jsx';
import Modal from 'react-bootstrap/Modal';
import Alert from 'react-bootstrap/Alert';

import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Select from 'react-select';
import { sprintf } from 'sprintf-js';
import VeloAce from '../core/ace.jsx';
import sanitize from '../core/sanitize.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import Col from 'react-bootstrap/Col';


import markdownit from 'markdown-it';

import './sigma-editor.css';

const escapeHTML = function(htmlStr) {
   let str = htmlStr || "";
   return str.replace(/&/g, "&amp;")
         .replace(/</g, "&lt;")
         .replace(/>/g, "&gt;")
         .replace(/"/g, "&quot;")
         .replace(/'/g, "&#39;");
};

class SigmaEditorDialog extends Component {
    static propTypes = {
        // VFS path to fetch the profile upload
        profile_components: PropTypes.array,
        selected_profile: PropTypes.string,
        notebook_id: PropTypes.string,
        cell: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchProfile();
    }

    selectedProfile = ()=>{
        return this.state.selected_profile || this.props.selected_profile;
    }

    state = {
        rule: "",
        selected_profile: "",
        selected_source: "",
        profiles: {},
    }

    fetchProfile = ()=>{
        if (!_.isEmpty(this.state.profiles)) {
            return;
        }

        let components = this.props.profile_components || [];
        if (_.isEmpty(components)) {
            return;
        }

        api.get_blob("v1/DownloadVFSFile", {
            offset: 0,
            length: 2000000,
            fs_components: components,
            org_id: window.globals.OrgId || "root",
        }, this.source.token).then(response => {
            if(response.error) {
                this.setState({error: true});

            } else {
                const view = new Uint8Array(response.data);
                let binary = '';
                var len = view.byteLength;
                for (var i = 0; i < len; i++) {
                    binary += String.fromCharCode( view[ i ] );
                }
                let json_obj = JSONparse(binary);
                let profiles = {};
                _.each(json_obj, (v,k)=>{
                    if (!_.isEmpty(v.Sources)) {
                        profiles[k] = {
                            FieldMappings: v.FieldMappings,
                            Description: v.Description,
                            Sources: v.Sources,
                        };
                    };
                });

                this.setState({profiles: profiles});
            }
        });
    }

    aceConfig = (ace) => {
        ace.completers = [this.getCompleter()];
        ace.setOptions({
            wrap: true,
            autoScrollEditorIntoView: true,
            enableLiveAutocompletion: true,
            enableBasicAutocompletion: true,
            minLines: 10,
            maxLines: 100000,
        });

        ace.resize();

        // Hold a reference to the ace editor.
        this.setState({ace: ace});
    };

    gcompletions = ()=>{
        let res = [
            {name: "New Rule",
             type: T("Rule"),
             description: T("Add a new Sigma Rule"),
             score: 1000,
             value: this.state.default_rule}];


        let modifiers = [
            {name: "re",
             description: T("Match any Regular Expression")},
            {name: "re_all",
             description: T("Match all Regular Expressions")},
            {name: "windash",
             description: T("Convert any provided command-line arguments or flags to use -")},
            {name: "base64",
             description: T("Encode the provided values as base64 encoded strings.")},
            {name: "base64offset",
             description: T("Used before contains to match base64 encoded strings (e.g. fieldname|base64offset|contains: )")},
            {name: "cidr",
             description: T("Allows for CIDR-formatted subnets to be used as field values.")},
            {name: "wide",
             description: T("encodes UTF16 - can only be used before base64 modifiers.")},
            {name: "gt",
             description: T("Greater than value.")},
            {name: "gte",
             description: T("Greater than or equal")},
            {name: "lt",
             description: T("Less than value.")},
            {name: "lte",
             description: T("Less than or equal value.")},
            {name: "vql",
             description: T("Evaluate value as arbitrary VQL Lambda.")},
            {name: "endswith",
             description: T("Field must end with value")},
            {name: "startswith",
             description: T("Field must start with value")},
            {name: "contains",
             description: T("Field must contain value (case insensitive)")},
            {name: "contains_all",
             description: T("Field must contain all values (case insensitive)")},
        ];

        _.each(modifiers, x=>{
            x.trigger= "|";
            x.type=T("Modifier");
            x.score=50;
            res.push(x);
        });

        // Add FieldMappings as completions
        let profiles = this.state.profiles || {};
        let selected_profile = profiles[this.selectedProfile()] || {};
        _.each(selected_profile.FieldMappings, (v, k)=>{
            res.push({name: k,
                      caption: sprintf("%s", v),
                      description: T("Field Mapping"),
                      type: T("Field Mapping")});
        });

        return res;
    }

    getCompleter = ()=>{
        return {
            // When the last part matches this, the completer kicks in. We
            // want it to triggr on ?
            identifierRegexps: [/[a-zA-Z_0-9.?$\-\u00A2-\uFFFF|]/],

            getCompletions: (editor, session, pos, prefix, callback) => {
                let completions = [];

                _.each(this.gcompletions(), x=>{
                    let item_name = x.name;
                    let snippet = x.name;

                    if (prefix === "?") {
                        item_name = prefix + item_name;

                    } else if(prefix.endsWith(x.trigger)) {
                        item_name = prefix + item_name;
                        snippet = item_name;

                    } else if(!item_name.startsWith(prefix)) {
                        return;
                    }

                    let caption = item_name;
                    if (x.caption) {
                        caption += " (" + x.caption + ")";
                    }

                    let completion = {
                        caption: caption,
                        description: T(x.description),
                        snippet: x.value || snippet,
                        type: x.description,
                        value: x.value || item_name,
                        meta: x.type || "",
                        score: x.score || 100,
                        docHTML: '<div class="arg-help">' +
                            escapeHTML(T(x.description)) + "</div>",
                    };

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
                });

                callback(null, completions);
            }
        };
    };


    setDefaultRule = logsource=>{
        let parts = logsource.split("/");
        let sigma_logsource = "";
        if (parts[0] && parts[0] != '*') {
            sigma_logsource += "\n category: " + parts[0];
        }

        if (parts[1] && parts[1] != '*') {
            sigma_logsource += "\n product: " + parts[1];
        }

        if (parts[2] && parts[2] != '*') {
            sigma_logsource += "\n service: " + parts[2];
        }

        let rule = sprintf(
`title: The rule title (should be unique)
logsource:%s

detection:
  # Add more selection clauses with different names.
  selection:
     # Use ? to see valid fields
     # Lists are exact matches with OR
     field_name:
       - value1
       - value2

     # Values are considered as AND
     field_name2: Value3

     # Modifiers follow | - Use ? to see valid modifiers
     field_name3|re: Regex

  # Condition can be a logical combination of selections (e.g. selection1 and selection2)
  condition: selection

# This contains a message that will be shown when matched. You can
# expand valid field names in the message.
details: "Field %%field_name%%"

# Add another rule below by pressing ? and selecting "New Rule"
---

`, sigma_logsource);

        let default_rule = sprintf(
`title: The rule title (should be unique)
logsource:%s

detection:
  selection:
     field_name:
       - value1
       - value2
  condition: selection

details: "Field %%field_name%%"

---

`,  sigma_logsource);
        this.setState({rule: rule, default_rule: default_rule});
    }

    renderSource = logsource=>{
        const md = markdownit();
        const result = md.render(logsource.description || "");
        return <>
                 <Card>
                   <Card.Header>
                     { this.state.selected_source }
                   </Card.Header>
                   <Card.Body>
                     { sanitize(result) }
                   </Card.Body>
                 </Card>
                 <div className="sigma-editor">
                   <VeloAce
                     text={this.state.rule}
                     mode="sigma"
                     aceConfig={this.aceConfig}
                     onChange={(x) => this.setState({rule: x})}
                   />
                 </div>
               </>;
    }


    selectSource = selected_source=>{
        // Only support global notebooks for now.
        let components = ["notebooks", this.props.notebook_id,
                          "attach", selected_source + ".yaml"];

        this.setState({selected_source: selected_source});

        // Try to load the rules from the server.
        api.get_blob("v1/DownloadVFSFile", {
            offset: 0,
            length: 2000000,
            fs_components: components,
            org_id: window.globals.OrgId || "root",
        }, this.source.token).then(response => {
            // If there is an error it is not really an error we just
            // dont have it yet.
            if(response.error) {
                this.setDefaultRule(selected_source);
            } else {
                const view = new Uint8Array(response.data);
                let rule = String.fromCharCode.apply(null, view);
                this.setState({rule: rule});
            }
        }).catch(res=>{
            this.setDefaultRule(selected_source);
        });
    }

    renderProfile = profile=>{
        let selected_source = profile.Sources[this.state.selected_source];
        let lines = (profile.Description || "").split("\n");
        let title = lines[0];

        return <>
                 <Alert variant="secondary">{title}</Alert>
                 <Form.Group as={Row}>
                   <Form.Label column sm="3">
                     <ToolTip tooltip={T("Log Source")}>
                       <div>
                         {T("Log Source")}
                       </div>
                     </ToolTip>
                   </Form.Label>
                   <Col sm="8">

                     <Select
                       className="sigma-profile-selector"
                       classNamePrefix="velo"
                       placeholder={T("Select Sigma Log Source")}
                       onChange={e=>this.selectSource(e.value)}
                       options={_.map(profile.Sources || {}, (v, k)=>{
                           let desc = "";
                           if (desc) {
                               desc = " ( " + desc + " ) ";
                           }
                           return {value: k, label: k + "        " + desc,
                                   isFixed: true, color: "#00B8D9"};
                       })}
                       spellCheck="false"
                     />
                   </Col>
                 </Form.Group>
                 {selected_source && this.renderSource(selected_source)}
                 </>;
    }

    saveRule = ()=>{
        api.post('v1/UploadNotebookAttachment', {
            data: btoa(this.state.rule),
            notebook_id: this.props.notebook_id,
            disable_attachment_id: true,
            cell_id: this.props.cell.cell_id,
            filename: this.state.selected_source + ".yaml",
            size: this.state.rule.length,
        }, this.source.token).then(response=>{

            // Save the rule then recalculate the cell to ensure we
            // get the correct sigma cells.
            api.post('v1/UpdateNotebookCell', {
                notebook_id: this.props.notebook_id,
                cell_id: this.props.cell.cell_id,
                type: this.props.cell.type || "Markdown",
                env: this.props.cell.env,
                currently_editing: false,
                input: this.props.cell.input,
            }, this.source.token).then(response=>{
                this.props.onClose();
            });
        });
    }

    render() {
        let profiles = this.state.profiles || {};
        let selected_profile_name = this.selectedProfile();
        let selected_profile = profiles[selected_profile_name] || {};
        let profile_desc = selected_profile.Description;


        return <Modal show={true}
                      enforceFocus={true}
                      scrollable={false}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Sigma Editor")}</Modal.Title>
                 </Modal.Header>
                 <Modal.Body className="sigma-editor-modal">
                   <Form.Group as={Row}>
                     <Form.Label column sm="3">
                       <ToolTip tooltip={T("Sigma Model")}>
                         <div>
                           {T("Sigma Model")}
                         </div>
                       </ToolTip>
                     </Form.Label>
                     <Col sm="8">
                       <Select
                         className="sigma-profile-selector"
                         classNamePrefix="velo"
                       placeholder={T("Select Sigma Profile")}
                       defaultValue={{value: selected_profile_name,
                                      label: selected_profile_name}}
                       onChange={(e)=>{
                           this.setState({selected_profile: e.value});
                       }}
                       options={_.map(this.state.profiles || {}, (v, k)=>{
                           return {value: k, label: k};
                       })}
                       spellCheck="false"
            />
        </Col>
                   </Form.Group>
                   {profile_desc && this.renderProfile(selected_profile) }

                 </Modal.Body>
                 <Modal.Footer>
                   <Button variant="secondary" onClick={this.props.onClose}>
                     {T("Close")}
                   </Button>
                   <Button variant="primary" onClick={this.saveRule}>
                     {T("Yes do it!")}
                   </Button>
                 </Modal.Footer>
               </Modal>;
    }
}


export default class VeloSigmaEditor extends Component {
    static propTypes = {
        notebook_id: PropTypes.string,
        cell: PropTypes.object,
        params: PropTypes.object,
    };

    state = {
        components: [],
        showSigmaDialog: false,
    }

    render() {
        let components = this.props.params && this.props.params.upload;
        let selected_profile = this.props.params &&
            this.props.params.selected_profile;
        return (
            <>
              <Button
                onClick={()=>this.setState({showSigmaDialog: true})}
                className="velo-sigma-editor">
                { T("Click to open the Sigma Editor") }
              </Button>
              { this.state.showSigmaDialog &&
                <SigmaEditorDialog
                  profile_components={components}
                  selected_profile={selected_profile}
                  notebook_id={this.props.notebook_id}
                  cell={this.props.cell}
                  onClose={()=>this.setState({showSigmaDialog: false})}
                />}
            </>
        );
    }
}
