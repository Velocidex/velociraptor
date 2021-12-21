import "./shell-viewer.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";
import _ from 'lodash';
import InputGroup from 'react-bootstrap/InputGroup';
import DropdownButton from 'react-bootstrap/DropdownButton';
import Dropdown from 'react-bootstrap/Dropdown';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { requestToParameters } from "../flows/utils.js";
import api from '../core/api-service.js';
import axios from 'axios';
import VeloTimestamp from "../utils/time.js";
import VeloPagedTable from "../core/paged-table.js";
import VeloAce from '../core/ace.js';
import Completer from '../artifacts/syntax.js';
import { DeleteFlowDialog } from "../flows/flows-list.js";


// Refresh every 5 seconds
const SHELL_POLL_TIME = 5000;

class VeloShellCell extends Component {
    static propTypes = {
        flow: PropTypes.object,
        client: PropTypes.object,
        fetchLastShellCollections: PropTypes.func.isRequired,
    }

    state = {
        collapsed: false,
        output: [],
        loaded: false,
        showDeleteWizard: false,
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    getInput = () => {
        if (!this.props.flow || !this.props.flow.request) {
            return "";
        }

        // Figure out the command we requested.
        var parameters = requestToParameters(this.props.flow.request);
        for (let k in parameters) {
            let params = parameters[k];
            let command = params.Command;
            if(_.isString(command)){
                return command;
            }
        }
        return "";
    }

    // Retrieve the flow result from the server and show it.
    loadData = () => {
        if (!this.props.flow || !this.props.flow.request ||
            !this.props.flow.request.artifacts) {
            return;
        };

        this.setState({"loaded": true});

        let artifact = this.props.flow.request.artifacts[0];

        api.get("v1/GetTable", {
            artifact: artifact,
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            rows: 500,
        }, this.source.token).then(function(response) {
            if (!response || !response.data || !response.data.rows) {
                return;
            };

            let data = [];
            for(var row=0; row<response.data.rows.length; row++) {
                var item = {};
                var current_row = response.data.rows[row].cell;
                for(var column=0; column<response.data.columns.length; column++) {
                    item[response.data.columns[column]] = current_row[column];
                }
                data.push(item);
            }
            this.setState({output: data, artifact: artifact});
        }.bind(this));
    };

    cancelFlow = (e) => {
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        api.post('v1/CancelFlow', {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
        }, this.source.token).then(function() {
            this.props.fetchLastShellCollections();
        }.bind(this));
    };

    render() {
        let buttons = [];

        // The cell can be collapsed ( inside an inset well) or
        // expanded. These buttons can switch between the two modes.
        if (this.state.collapsed) {
            buttons.push(
                <button className="btn btn-default" key={1}
                        title="Expand"
                        onClick={() => this.setState({collapsed: false})} >
                  <i><FontAwesomeIcon icon="expand"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn btn-default" key={2}
                        title="Collapse"
                        onClick={() => this.setState({collapsed: true})} >
                  <i><FontAwesomeIcon icon="compress"/></i>
                </button>
            );
        }

        // Button to load the output from the server (it could be
        // large so we dont fetch it until the user asks)
        if (this.state.loaded) {
            buttons.push(
                <button className="btn btn-default" key={3}
                        title="Hide Output"
                        onClick={() => this.setState({"loaded": false})} >
                  <i><FontAwesomeIcon icon="eye-slash"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn btn-default" key={4}
                        title="Load Output"
                        onClick={this.loadData}>
                  <i><FontAwesomeIcon icon="eye"/></i>
                </button>
            );
        }

        // If the flow is currently running we may be able to stop it.
        if (this.props.flow.state  === 'RUNNING') {
            buttons.push(
                <button className="btn btn-default" key={5}
                        title="Stop"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </button>
            );
        }

        let flow_status = [];
        if (this.props.flow.state  === 'RUNNING') {
            flow_status.push(
                <button className="btn btn-outline-info" key={6}
                        disabled>
                  <i><FontAwesomeIcon icon="spinner" spin /></i>
                  <VeloTimestamp usec={this.props.flow.create_time/1000} />
                by {this.props.flow.request.creator}
                </button>
            );
        } else if (this.props.flow.state  === 'FINISHED') {
            flow_status.push(
                <button className="btn btn-outline-info" key={7}
                        disabled>
                  <VeloTimestamp usec={this.props.flow.active_time/1000} />
                  by {this.props.flow.request.creator}
                </button>
            );

        } else if (this.props.flow.state  === 'ERROR') {
            flow_status.push(
                <button className="btn btn-outline-info" key={8}
                        disabled>
                  <i><FontAwesomeIcon icon="exclamation"/></i>
                <VeloTimestamp usec={this.props.flow.create_time/1000} />
            by {this.props.flow.request.creator}
                </button>
            );
        }

        flow_status.push(
            <button className="btn btn-default" key={9}
                    title="Delete"
                    onClick={()=>this.setState({showDeleteWizard: true})}>
              <i><FontAwesomeIcon icon="trash"/></i>
            </button>
        );


        let output = "";
        if (this.state.loaded) {
            output = this.state.output.map((item, index) => {
                return <div className='notebook-output' key={index} >
                         <pre> {item.Stdout} </pre>
                       </div>;
            });
        }

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flow={this.props.flow}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                  }}
                />
              }
              <div className={classNames({
                       collapsed: this.state.collapsed,
                       expanded: !this.state.collapsed,
                       'shell-cell': true,
                   })}>

                <div className='notebook-input'>
                  <div className="cell-toolbar">
                    <div className="btn-group" role="group">
                      { buttons }
                    </div>
                    <div className="btn-group float-right" role="group">
                      { flow_status }
                    </div>
                  </div>

                  <pre> { this.getInput() } </pre>
                </div>
                {output}
              </div>
            </>
        );
    };
};



class VeloVQLCell extends Component {
    static propTypes = {
        flow: PropTypes.object,
        client: PropTypes.object,
        fetchLastShellCollections: PropTypes.func.isRequired,
    }

    state = {
        loaded: false,
        showDeleteWizard: false,
    }

    getInput = () => {
        if (!this.props.flow || !this.props.flow.request) {
            return "";
        }

        // Figure out the command we requested.
        var parameters = requestToParameters(this.props.flow.request);
        for (let k in parameters) {
            let params = parameters[k];
            let command = params.Command;
            if(_.isString(command)){
                return command;
            }
        }
        return "";
    }

    cancelFlow = (e) => {
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        api.post('v1/CancelFlow', {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
        }, this.source.token).then(function() {
            this.props.fetchLastShellCollections();
        }.bind(this));
    };

    aceConfig = (ace) => {
        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25,
            placeholder: "Type VQL to run on the client",
            readOnly: true,
        });

        this.setState({ace: ace});
    };

    render() {
        let buttons = [];

        // The cell can be collapsed ( inside an inset well) or
        // expanded. These buttons can switch between the two modes.
        if (this.state.collapsed) {
            buttons.push(
                <button className="btn btn-default" key={1}
                        title="Expand"
                        onClick={() => this.setState({collapsed: false})} >
                  <i><FontAwesomeIcon icon="expand"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn btn-default" key={2}
                        title="Collapse"
                        onClick={() => this.setState({collapsed: true})} >
                  <i><FontAwesomeIcon icon="compress"/></i>
                </button>
            );
        }

        // Button to load the output from the server (it could be
        // large so we dont fetch it until the user asks)
        if (this.state.loaded) {
            buttons.push(
                <button className="btn btn-default" key={3}
                        title="Hide Output"
                        onClick={() => this.setState({"loaded": false})} >
                  <i><FontAwesomeIcon icon="eye-slash"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn btn-default" key={4}
                        title="Load Output"
                        onClick={() => this.setState({"loaded": true})}>
                  <i><FontAwesomeIcon icon="eye"/></i>
                </button>
            );
        }

        // If the flow is currently running we may be able to stop it.
        if (this.props.flow.state  === 'RUNNING') {
            buttons.push(
                <button className="btn btn-default" key={5}
                        title="Stop"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </button>
            );
        }

        let flow_status = [];
        if (this.props.flow.state  === 'RUNNING') {
            flow_status.push(
                <button className="btn btn-outline-info" key={15}
                        disabled>
                  <i><FontAwesomeIcon icon="spinner" spin /></i>
                  <VeloTimestamp usec={this.props.flow.create_time/1000} />
                by {this.props.flow.request.creator}
                </button>
            );
        } else if (this.props.flow.state  === 'FINISHED') {
            flow_status.push(
                <button className="btn btn-outline-info" key={12}
                        disabled>
                  <VeloTimestamp usec={this.props.flow.active_time/1000} />
                  by {this.props.flow.request.creator}
                </button>
            );

        } else if (this.props.flow.state  === 'ERROR') {
            flow_status.push(
                <button className="btn btn-outline-info" key={13}
                        disabled>
                  <i><FontAwesomeIcon icon="exclamation"/></i>
                <VeloTimestamp usec={this.props.flow.create_time/1000} />
            by {this.props.flow.request.creator}
                </button>
            );
        }

        flow_status.push(
            <button className="btn btn-default" key={5}
                    title="Delete"
                    onClick={()=>this.setState({showDeleteWizard: true})}>
              <i><FontAwesomeIcon icon="trash"/></i>
            </button>
        );

        let output = <div></div>;
        if (this.state.loaded) {
            let artifact = this.props.flow && this.props.flow.request &&
                this.props.flow.request.artifacts && this.props.flow.request.artifacts[0];
            let params = {
                artifact: artifact,
                client_id: this.props.flow.client_id,
                flow_id: this.props.flow.session_id,
            };
            output = <VeloPagedTable params={params} />;
        }

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flow={this.props.flow}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                  }}
                />
              }
              <div className={classNames({
                       collapsed: this.state.collapsed,
                       expanded: !this.state.collapsed,
                       'shell-cell': true,
                   })}>

                <div className='notebook-input'>
                  <div className="cell-toolbar">
                    <div className="btn-group" role="group">
                      { buttons }
                    </div>
                    <div className="btn-group float-right" role="group">
                      { flow_status }
                    </div>
                  </div>

                  <VeloAce text={this.getInput()} mode="sql"
                           aceConfig={this.aceConfig}
                  />
                </div>
                {output}
              </div>
            </>
        );
    };
};



class ShellViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        default_shell: PropTypes.object,
    }

    constructor(props) {
        super(props);
        this.state = {
            flows: [],
            shell_type: props.default_shell || 'Powershell',
            command: "",
        };
    }

    componentDidMount() {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchLastShellCollections, SHELL_POLL_TIME);
        this.fetchLastShellCollections();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    // Force the flows list to update as soon as the client changes
    // instead of waiting to the next poll.
    componentDidUpdate(prevProps, prevState, snapshot) {
        var new_client_id = this.props.client && this.props.client.client_id;
        var old_client_id = prevProps.client && prevProps.client.client_id;

        if (new_client_id !== old_client_id) {
            this.fetchLastShellCollections();
        }
    }

    setType = (shell) => {
        this.setState({
            shell_type: shell,
        });
    };

    setText = (e) => {
        this.setState({
            command: e.target.value,
        });
    };

    fetchLastShellCollections = () => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        api.get('v1/GetClientFlows/'+ this.props.client.client_id, {
            count: 100, offset: 0,
            artifact: "(Windows.System.PowerShell|Windows.System.CmdShell|Linux.Sys.BashShell|Generic.Client.VQL)"
        },
                this.source.token // CancelToken
               ).then(function(response) {
                   if (response.cancel) return;
                   let new_state  = Object.assign({}, this.state);
                   new_state.flows = [];
                   if (!response.data) {
                       return;
                   }

                   let items = response.data.items || [];

                   for(var i=0; i<items.length; i++) {
                       var artifacts = items[i].request.artifacts;
                       for (var j=0; j<artifacts.length; j++) {
                           var artifact = artifacts[j];
                           if (artifact === "Windows.System.PowerShell" ||
                               artifact === "Windows.System.CmdShell" ||
                               artifact === "Generic.Client.VQL" ||
                               artifact === "Linux.Sys.BashShell" ) {
                               new_state.flows.push(items[i]);
                           }
                       }
                   };

                   this.setState(new_state);
               }.bind(this), function(){});
    };

    launchCommand = () => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        var artifact = "";
        if (this.state.shell_type.toLowerCase() === "powershell") {
            artifact = "Windows.System.PowerShell";
        } else if(this.state.shell_type.toLowerCase() === "cmd") {
            artifact = "Windows.System.CmdShell";
        } else if(this.state.shell_type.toLowerCase() === "bash") {
            artifact = "Linux.Sys.BashShell";
        } else if(this.state.shell_type.toLowerCase() === "vql") {
            artifact = "Generic.Client.VQL";
        } else {
            return;
        };

        var params = {
            client_id: this.props.client.client_id,
            artifacts: [artifact],
            specs: [{
                artifact: artifact,
                parameters: {
                    env: [{key: "Command", value: this.state.command}],
                }
            }],
            urgent: true,
        };

        api.post('v1/CollectArtifact', params).then(response=>{
            // Refresh the artifacts immediately.
            this.fetchLastShellCollections();
        }, this.source.token);
    };

    renderCells(flows) {
        return flows.map((flow, index) => {
            let artifact = flow && flow.request &&  flow.request.artifacts &&
                flow.request.artifacts[0];
            if (artifact === "Generic.Client.VQL") {
                return <VeloVQLCell key={flow.session_id}
                                    fetchLastShellCollections={this.fetchLastShellCollections}
                                    flow={flow} client={this.props.client} />;
            };

            return (
                <VeloShellCell key={flow.session_id}
                               fetchLastShellCollections={this.fetchLastShellCollections}
                               flow={flow} client={this.props.client} />
            );
        });
    };

    aceConfig = (ace) => {
        // Attach a completer to ACE.
        let completer = new Completer();
        completer.initializeAceEditor(ace, {});

        ace.setOptions({
            autoScrollEditorIntoView: true,
            placeholder: "Run VQL on client",
            maxLines: 25
        });

        this.setState({ace: ace});
    };

    render() {
        let simple_textarea = true;
        if (this.state.shell_type === "VQL") {
            simple_textarea = false;
        }


        return (
            <>
              <div className="shell-command">
                <InputGroup className="mb-3 d-flex">
                  <InputGroup.Prepend>
                    <DropdownButton as={InputGroup}
                                    title={this.state.shell_type}
                                    onSelect={(e) => this.setType(e)}
                                    id="bg-nested-dropdown">
                      <Dropdown.Item eventKey="Powershell">Powershell</Dropdown.Item>
                      <Dropdown.Item eventKey="Cmd">Cmd</Dropdown.Item>
                      <Dropdown.Item eventKey="Bash">Bash</Dropdown.Item>
                      <Dropdown.Item eventKey="VQL">VQL</Dropdown.Item>
                    </DropdownButton>
                  </InputGroup.Prepend>
                  { simple_textarea ?
                    <textarea focus-me="controller.focus" rows="1"
                              className="form-control"
                              placeholder="Run command on client"
                              value={this.state.command}
                              onChange={(e) => this.setText(e)}>
                    </textarea> :
                    <VeloAce
                      mode="VQL"
                      aceConfig={this.aceConfig}
                      text={this.state.command}
                      onChange={(value) => {this.setState({command: value});}}
                      commands={[{
                          name: 'saveAndExit',
                          bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
                          exec: (editor) => {
                              this.launchCommand();
                          },
                      }]}
                    />
                  }

                  <InputGroup.Append className="input-group-append">
                    <button className="btn btn-danger" type="button"
                            disabled={!this.state.command}
                            onClick={this.launchCommand}
                    >Launch</button>
                  </InputGroup.Append>
                </InputGroup>
              </div>
              <div className="shell-results">
                { this.renderCells(this.state.flows) }
              </div>
            </>
        );
    };
}

export default ShellViewer;
