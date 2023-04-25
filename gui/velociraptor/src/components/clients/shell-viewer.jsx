import "./shell-viewer.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";
import _ from 'lodash';
import InputGroup from 'react-bootstrap/InputGroup';
import DropdownButton from 'react-bootstrap/DropdownButton';
import Dropdown from 'react-bootstrap/Dropdown';
import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { requestToParameters } from "../flows/utils.jsx";
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import VeloTimestamp from "../utils/time.jsx";
import VeloPagedTable from "../core/paged-table.jsx";
import VeloAce from '../core/ace.jsx';
import Completer from '../artifacts/syntax.jsx';
import { DeleteFlowDialog } from "../flows/flows-list.jsx";
import Button from 'react-bootstrap/Button';
import { withRouter }  from "react-router-dom";

// Refresh every 5 seconds
const SHELL_POLL_TIME = 5000;

class _VeloShellCell extends Component {
    static propTypes = {
        flow: PropTypes.object,
        client: PropTypes.object,
        fetchLastShellCollections: PropTypes.func.isRequired,

        // React router props.
        history: PropTypes.object,
    }

    state = {
        collapsed: false,
        output: [],
        loaded: false,
        showDeleteWizard: false,
    }

    componentDidMount() {
        this.source = CancelToken.source();
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

    viewFlow = target=>{
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        let client_id = this.props.flow.client_id;
        let session_id = this.props.flow.session_id;
        this.props.history.push('/collected/' + client_id +
                                "/" + session_id + "/" + target);
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
                <button className="btn-tooltip btn btn-default" key={1}
                        data-tooltip={T("Expand")}
                        data-position="right"
                        onClick={() => this.setState({collapsed: false})} >
                  <i><FontAwesomeIcon icon="expand"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={2}
                        data-tooltip={T("Collapse")}
                        data-position="right"
                        onClick={() => this.setState({collapsed: true})} >
                  <i><FontAwesomeIcon icon="compress"/></i>
                </button>
            );
        }

        // Button to load the output from the server (it could be
        // large so we dont fetch it until the user asks)
        if (this.state.loaded) {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={3}
                        data-tooltip={T("Hide Output")}
                        data-position="right"
                        onClick={() => this.setState({"loaded": false})} >
                  <i><FontAwesomeIcon icon="eye-slash"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={4}
                        data-tooltip={T("Load Output")}
                        data-position="right"
                        onClick={this.loadData}>
                  <i><FontAwesomeIcon icon="eye"/></i>
                </button>
            );
        }

        let flow_status = [
            <button className="btn btn-outline-info" key="1"
              onClick={e=>{this.viewFlow("overview");}}
            >
              <i><FontAwesomeIcon icon="external-link-alt"/></i>
            </button>];

        // If the flow is currently running we may be able to stop it.
        if (this.props.flow.state  === 'RUNNING') {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={5}
                        data-tooltip={T("Stop")}
                        data-position="right"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </button>
            );

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
            <button className="btn-tooltip btn btn-default" key={9}
                    data-tooltip={T("Delete")}
                    data-position="right"
                    onClick={()=>this.setState({showDeleteWizard: true})}>
              <i><FontAwesomeIcon icon="trash"/></i>
            </button>
        );


        let output = "";
        if (this.state.loaded) {
            output = [this.state.output.map((item, index) => {
                return <div className='notebook-output' key={index} >
                         <pre> {item.Stdout} </pre>
                       </div>;
            })];

            if (this.props.flow.state  === 'ERROR') {
                output.push(<Button variant="danger" key="errors"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" block>
                              {T('Error')}
                            </Button>);
            } else {
                output.push(<Button variant="link" key="logs"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" block>
                              {T('Logs')}
                            </Button>);
            }

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

const VeloShellCell = withRouter(_VeloShellCell);

class _VeloVQLCell extends Component {
    static propTypes = {
        flow: PropTypes.object,
        client: PropTypes.object,
        fetchLastShellCollections: PropTypes.func.isRequired,

        // React router props.
        history: PropTypes.object,
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

    viewFlow = target=>{
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        let client_id = this.props.flow.client_id;
        let session_id = this.props.flow.session_id;
        this.props.history.push('/collected/' + client_id +
                                "/" + session_id + "/" + target);
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
            placeholder: T("Type VQL to run on the client"),
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
                <button className="btn-tooltip btn btn-default" key={1}
                        data-tooltip={T("Expand")}
                        data-position="right"
                        onClick={() => this.setState({collapsed: false})} >
                  <i><FontAwesomeIcon icon="expand"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={2}
                        data-tooltip={T("Collapse")}
                        data-position="right"
                        onClick={() => this.setState({collapsed: true})} >
                  <i><FontAwesomeIcon icon="compress"/></i>
                </button>
            );
        }

        // Button to load the output from the server (it could be
        // large so we dont fetch it until the user asks)
        if (this.state.loaded) {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={3}
                        data-tooltip={T("Hide Output")}
                        data-position="right"
                        onClick={() => this.setState({"loaded": false})} >
                  <i><FontAwesomeIcon icon="eye-slash"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={4}
                        data-tooltip={T("Load Output")}
                        data-position="right"
                        onClick={() => this.setState({"loaded": true})}>
                  <i><FontAwesomeIcon icon="eye"/></i>
                </button>
            );
        }

        let flow_status = [
            <button className="btn btn-outline-info" key={0}
              onClick={e=>{this.viewFlow("overview");}}
            >
              <i><FontAwesomeIcon icon="external-link-alt"/></i>
            </button>];

        // If the flow is currently running we may be able to stop it.
        if (this.props.flow.state  === 'RUNNING') {
            buttons.push(
                <button className="btn-tooltip btn btn-default" key={5}
                        data-tooltip={T("Stop")}
                        data-position="right"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </button>
            );

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
            <button className="btn-tooltip btn btn-default" key={50}
                    data-tooltip={T("Delete")}
                    data-position="right"
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
            output = [<VeloPagedTable params={params} key={0} />];

            if (this.props.flow.state  === 'ERROR') {
                output.push(<Button variant="danger" key="ERROR"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" block>
                              {T('Error')}
                            </Button>);
            } else {
                output.push(<Button variant="link" key="Logs"
                                    onClick={e=>{this.viewFlow("logs");}}
                                    size="lg" block>
                              {T('Logs')}
                            </Button>);
            }
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

const VeloVQLCell = withRouter(_VeloVQLCell);

class ShellViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
        default_shell: PropTypes.string,
    }

    constructor(props) {
        super(props);
        this.state = {
            flows: [],
            shell_type: props.default_shell || 'Powershell',
            command: "",
        };

        this.state.client_os = props.client && props.client.os_info &&
            props.client.os_info.system;

        // Powershell can exist on Linux/MacOS but Bash is a more reasonable default
        if (this.state.client_os && this.state.client_os !== "windows") {
            this.state.shell_type = 'Bash';
        }
    }

    componentDidMount() {
        this.source = CancelToken.source();
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

        let client_id = this.props.client.client_id;
        api.get('v1/GetClientFlows/'+ client_id, {
            count: 100, offset: 0,
            artifact: "(Windows.System.PowerShell|Windows.System.CmdShell|Linux.Sys.BashShell|Generic.Client.VQL)"
        },
                this.source.token // CancelToken
               ).then(function(response) {
                   if (response.cancel) return;
                    if (!response.data) {
                        return;
                    }

                   let new_state  = Object.assign({}, this.state);
                   new_state.flows = [];
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
                return <VeloVQLCell key={index}
                                    fetchLastShellCollections={this.fetchLastShellCollections}
                                    flow={flow} client={this.props.client} />;
            };

            return (
                <VeloShellCell key={index}
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
            placeholder: T("Run VQL on client"),
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
                      { (!this.state.client_os || this.state.client_os === "windows") &&
                        <Dropdown.Item eventKey="Cmd">Cmd</Dropdown.Item> }
                      { (!this.state.client_os || this.state.client_os !== "windows") &&
                        <Dropdown.Item eventKey="Bash">Bash</Dropdown.Item> }
                      <Dropdown.Item eventKey="VQL">VQL</Dropdown.Item>
                    </DropdownButton>
                  </InputGroup.Prepend>
                  { simple_textarea ?
                    <textarea focus-me="controller.focus" rows="1"
                              className="form-control"
                              placeholder={T("Run command on client")}
                              spellCheck="false"
                              value={this.state.command}
                              onChange={(e) => this.setText(e)}>
                    </textarea> :
                    <VeloAce
                      mode="VQL"
                      className="vql-shell-input"
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
