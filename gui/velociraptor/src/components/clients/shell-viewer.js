import "./shell-viewer.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import classNames from "classnames";

import InputGroup from 'react-bootstrap/InputGroup';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import DropdownButton from 'react-bootstrap/DropdownButton';
import Dropdown from 'react-bootstrap/Dropdown';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';
import axios from 'axios';
import VeloTimestamp from "../utils/time.js";

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
    }

    set = (k, v) => {
        let new_state  = Object.assign({}, this.state);
        new_state[k] = v;
        this.setState(new_state);
    }

    getInput = () => {
        if (!this.props.flow || !this.props.flow.request ||
            !this.props.flow.request.parameters ||
            !this.props.flow.request.parameters.env) {
            return "";
        }

        // Figure out the command we requested.
        var parameters = this.props.flow.request.parameters.env;
        for(var i=0; i<parameters.length; i++) {
            if (parameters[i].key == 'Command') {
                return parameters[i].value;
            };
        }
        return "";
    }

    // Retrieve the flow result from the server and show it.
    loadData = () => {
        if (!this.props.flow || !this.props.flow.request ||
            !this.props.flow.request.artifacts) {
            return;
        };

        this.set("loaded", true);

        let artifact = this.props.flow.request.artifacts[0];

        api.get("api/v1/GetTable", {
            artifact: artifact,
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            rows: 500,
        }).then(function(response) {
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
            this.set("output", data);
        }.bind(this));
    };

    cancelFlow = (e) => {
        if (!this.props.flow || !this.props.flow.session_id ||
            !this.props.flow.client_id) {
            return;
        }

        api.post('api/v1/CancelFlow', {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
        }).then(function() {
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
                        onClick={() => this.set("collapsed", false)} >
                  <i><FontAwesomeIcon icon="expand"/></i>
                </button>
            );
        } else {
            buttons.push(
                <button className="btn btn-default" key={2}
                        title="Collapse"
                        onClick={() => this.set("collapsed", true)} >
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
                        onClick={() => this.set("loaded", false)} >
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
        if (this.props.flow.state  == 'RUNNING') {
            buttons.push(
                <button className="btn btn-default" key={5}
                        title="Stop"
                        onClick={this.cancelFlow}>
                  <i><FontAwesomeIcon icon="stop"/></i>
                </button>
            );
        }

        let flow_status;
        if (this.props.flow.state  == 'RUNNING') {
            flow_status = (
                <button className="btn btn-outline-info"
                        disabled>
                  <i><FontAwesomeIcon icon="spinner" spin /></i>
                  <VeloTimestamp usec={this.props.flow.create_time/1000} />
                by {this.props.flow.request.creator}
                </button>
            );
        } else if (this.props.flow.state  == 'FINISHED') {
            flow_status = (
                <button className="btn btn-outline-info"
                        disabled>
                  <VeloTimestamp usec={this.props.flow.active_time/1000} />
                  by {this.props.flow.request.creator}
                </button>
            );

        } else if (this.props.flow.state  == 'ERROR') {
            flow_status = (
                <button className="btn btn-outline-info"
                        disabled>
                  <i><FontAwesomeIcon icon="exclamation"/></i>
                <VeloTimestamp usec={this.props.flow.create_time/1000} />
            by {this.props.flow.request.creator}
                </button>
            );
        }

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


class ShellViewer extends Component {
    static propTypes = {
        client: PropTypes.object,
    }

    constructor(props) {
        super(props);
        this.state = {
            flows: [],
            shell_type: 'Powershell',
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
            command: this.state.command,
        });
    };

    setText = (e) => {
        this.setState({
            shell_type: this.state.shell_type,
            command: e.target.value,
        });
    };

    fetchLastShellCollections = () => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        var url = 'api/v1/GetClientFlows/' + this.props.client.client_id;

        api.get(url, {
            count: 100, offset: 0},
                this.source.token // CancelToken
               ).then(
                   function(response) {
                       let new_state  = Object.assign({}, this.state);
                       new_state.flows = [];
                       if (!response.data || !response.data.items) {
                           return;
                       }

                       let items = response.data.items;

                       for(var i=0; i<items.length; i++) {
                           var artifacts = items[i].request.artifacts;
                           for (var j=0; j<artifacts.length; j++) {
                               var artifact = artifacts[j];
                               if (artifact === "Windows.System.PowerShell" ||
                                   artifact === "Windows.System.CmdShell" ||
                                   artifact === "Linux.Sys.BashShell" ) {
                                   new_state.flows.push(items[i]);
                               }
                           }
                       };

                       this.setState(new_state);
                   }.bind(this),
                   function(){},
               );
    };

    launchCommand = () => {
        if (!this.props.client || !this.props.client.client_id) {
            return;
        }

        var artifact = "";
        if (this.state.shell_type == "Powershell") {
            artifact = "Windows.System.PowerShell";
        } else if(this.state.shell_type == "Cmd") {
            artifact = "Windows.System.CmdShell";
        } else if(this.state.shell_type == "Bash") {
            artifact = "Linux.Sys.BashShell";
        } else {
            return;
        };

        var params = {
            client_id: this.props.client.client_id,
            artifacts: [artifact],
            parameters: {
                env: [{key: "Command", value: this.state.command}],
            },
        };

        api.post('api/v1/CollectArtifact', params).then(
            function success(response) {
                // Refresh the artifacts immediately.
                this.fetchLastShellCollections();
            }.bind(this),
            function failure(response) {
                this.responseError = response['data']['error'] || 'Unknown error';
            }.bind(this));
    };

    renderCells(flows) {
        return flows.map((flow, index) => {
            return (
                <VeloShellCell key={flow.session_id}
                               fetchLastShellCollections={this.fetchLastShellCollections}
                               flow={flow} client={this.props.client} />
            );
        });
    };

    render() {
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
                    </DropdownButton>
                  </InputGroup.Prepend>
                  <textarea focus-me="controller.focus" rows="1"
                            className="form-control"
                            onChange={(e) => this.setText(e)}>
                  </textarea>

                  <InputGroup.Append className="input-group-append">
                    <button className="btn btn-danger" type="button"
                            disabled={!this.state.command}
                            onClick={this.launchCommand}
                    >Launch</button>
                  </InputGroup.Append>
                </InputGroup>
              </div>

              { this.renderCells(this.state.flows) }
            </>
        );
    };
}

export default ShellViewer;
