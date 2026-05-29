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
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import VeloTimestamp from "../utils/time.jsx";
import VeloPagedTable from "../core/paged-table.jsx";
import VeloAce from '../core/ace.jsx';
import Completer from '../artifacts/syntax.jsx';
import { DeleteFlowDialog } from "../flows/flows-list.jsx";
import Button from 'react-bootstrap/Button';
import { withRouter, Link }  from "react-router-dom";
import { JSONparse } from '../utils/json_parse.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';
import { parseTableResponse } from '../utils/table.jsx';

import Accordion from 'react-bootstrap/Accordion';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import  ButtonToolbar from 'react-bootstrap/ButtonToolbar';
import  ButtonGroup from 'react-bootstrap/ButtonGroup';
import Form from 'react-bootstrap/Form';
import Card from 'react-bootstrap/Card';

// Refresh every 5 seconds
const SHELL_POLL_TIME = 5000;

const getTime = (x)=>{
    try{
        let d = new Date(x);
        return d.getTime();
    } catch(e) {
        return 0;
    }
}


class _VeloShellCell extends Component {
    static propTypes = {
        flow_id: PropTypes.string,
        artifact: PropTypes.string,
        client_id: PropTypes.string,
        fetchLastShellCollections: PropTypes.func.isRequired,

        // Signify we are in full screen mode.
        fullscreen: PropTypes.bool,

        // React router props.
        history: PropTypes.object,
    }

    state = {
        // Indicates if the view cell is collapsed to limited hight or
        // unlimited. Controlled by collapse/expand button.
        collapsed: false,
        output: [],

        // Indicates if the cell is loaded (has data) or
        // unloaded. Controlled by the hide/show button.
        loaded: false,
        showDeleteWizard: false,
        activeRows: {},
        enabled: true,

        flow: {},
    }

    constructor(props) {
        super(props);
        this.scrollRef = React.createRef();
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.interval = setInterval(this.loadData, SHELL_POLL_TIME);
        if(this.props.fullscreen) {
            this.setState({loaded: true});
            this.loadData(true);
            return;
        }
        this.loadData();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    setFullScreen = () => {
        let client_id = this.props.client_id;
        let selected_flow = this.props.flow_id;
        let artifact = this.props.artifact;

        if (client_id && selected_flow && artifact) {
            return "/fullscreen/shell/" + artifact +"/" + client_id + "/" +
                    selected_flow;
        }
        return "";
    }

    transcriptLink = () => {
        let client_id = this.props.client_id;
        let selected_flow = this.props.flow_id;
        if (client_id && selected_flow) {
            let link = "/fullscreen/collected/" + client_id + "/" +
                selected_flow + "/notebook";

            return <ToolTip tooltip={T("Transcript")} key="transcript">
                     <Link to={link}
                           target="_blank" rel="noopener noreferrer"
                           role="button" className="btn btn-default">
                       <FontAwesomeIcon icon="terminal"/>
                       <span className="sr-only">{T("Full Screen")}</span>
                     </Link>
                   </ToolTip>;
        }
        return <></>;
    }

    viewFlow = target=>{
        let client_id = this.props.client_id;
        let flow_id = this.props.flow_id;

        if (!flow_id || !client_id) {
            return "";
        }

        return '/collected/' + client_id + "/" + flow_id + "/" + target;
    }

    // Retrieve the flow result from the server and show it.
    loadData = (load_now) => {
        if (!this.props.flow_id || !this.props.client_id ||
            !this.props.artifact) {
            return;
        };

        // Get the flow status
        api.get("v1/GetFlowDetails", {
            client_id: this.props.client_id,
            flow_id: this.props.flow_id,
            include_full_request: true,
        }, this.source.token).then((response) => {
            let context = response.data.context;
            if(!_.isEqual(this.state.flow, context)) {
                this.setState({flow: context});
            }
        });

        // Don't fetch anything until the user unfolds the view.
        if (!load_now && !this.state.loaded) {
            return;
        }

        let artifact = this.props.artifact;
        api.get("v1/GetTable", {
            artifact: artifact,
            client_id: this.props.client_id,
            flow_id: this.props.flow_id,
            rows: 5000,
        }, this.source.token).then(response=>{
            if (!response || !response.data || !response.data.rows) {
                return;
            };
            let data = parseTableResponse(response);

            // Only scroll if the length changes.
            if(this.scrollRef.current &&
               this.state.last_length != data.length) {
                this.scrollToBottom();
            }

            // Do not update the state un necessarily
            if(this.state.last_length != data.length) {
                this.setState({output: data,
                               last_length: data.length,
                               artifact: artifact});
            }
        });
    };

    scrollToBottom = ()=>{
        // Inside setTimeout so we can run it outside the render
        // cycle.
        setTimeout(()=>{
            if(this.scrollRef.current) {
                this.scrollRef.current.scrollIntoView({
                    block: 'end',
                    inline: 'nearest',
                    behavior: "smooth",
                });
            }
        }, 100);
    }

    cancelFlow = (e) => {
        if (!this.props.flow_id || !this.props.client_id) {
            return;
        }

        api.post('v1/CancelFlow', {
            client_id: this.props.client_id,
            flow_id: this.props.flow_id,
        }, this.source.token).then(function() {
            this.props.fetchLastShellCollections();
        }.bind(this));
    };

    // Launch the command within this shell session.
    launchCommand = e=>{
        if (!this.props.flow_id || !this.props.client_id ||
            !this.state.command) {
            return;
        }

        var params = {
            client_id: this.props.client_id,
            flow_id: this.props.flow_id + "/" + Date.now(),
            artifacts: [this.props.artifact],
            specs: [{
                artifact: this.props.artifact,
                parameters: {
                    env: [{key: "Command", value: this.state.command}],
                },
                // Prioritise sending the rows quickly
                max_batch_wait: 1,
                max_batch_rows: 1,
            }],
            urgent: true,
        };

        this.setState({enabled: false});
        api.post('v1/CollectArtifact', params).then(response=>{
            // Refresh the artifacts immediately.
            this.props.fetchLastShellCollections();
            this.scrollToBottom();
            // Block the launch button for a while for debouncing.
            setTimeout(()=>{
                this.setState({enabled: true});
            }, 1000);

        }, this.source.token);

        // Clear the command after launching it.
        this.setState({command: ""});
    }

    renderCollapsed = ()=>{
        if (this.state.collapsed) {
            return (
                <ToolTip tooltip={T("Expand")} key="expand">
                  <Button variant="default"
                          onClick={() => this.setState({collapsed: false})} >
                    <i><FontAwesomeIcon icon="expand"/></i>
                  </Button>
                </ToolTip>
            );
        } else {
            return (
                <ToolTip tooltip={T("Collapse")} key="expand2">
                  <Button variant="default"
                          onClick={() => this.setState({collapsed: true})} >
                    <i><FontAwesomeIcon icon="compress"/></i>
                  </Button>
                </ToolTip>
            );
        }
    }

    renderLoadedButtons = ()=>{
        if (this.state.loaded) {
            return (
                <ToolTip tooltip={T("Hide Output")} key="loaded">
                  <Button variant="default"
                          onClick={() => this.setState({"loaded": false})} >
                    <span className="icon-small">
                      { this.props.artifact}
                    </span>
                    <i><FontAwesomeIcon icon="eye-slash"/></i>
                  </Button>
                </ToolTip>
            );
        } else {
            return (
                <ToolTip tooltip={T("Load Output")} key="loaded2">
                  <Button variant="default"
                          onClick={()=>{
                              this.setState({loaded:true});
                              this.loadData(true);
                          }}>
                    <span className="icon-small">
                      { this.props.artifact}
                    </span>
                    <i><FontAwesomeIcon icon="eye"/></i>
                  </Button>
                </ToolTip>
            );
        }
    }

    renderExpandAll = ()=>{
        if(_.isEmpty(this.state.activeRows)) {
            return (
                <ToolTip tooltip={T("Show all")} key="show all">
                  <Button variant="default"
                          disabled={!this.state.loaded}
                          onClick={() => this.setState({
                              activeRows: _.map(this.state.output, (x, idx)=>idx),
                          })} >
                    <i><FontAwesomeIcon icon="angle-down"/></i>
                  </Button>
                </ToolTip>
            );
        }

        return (
            <ToolTip tooltip={T("Hide all")} key="hide all">
              <Button variant="default"
                      disabled={!this.state.loaded}
                      onClick={() => this.setState({activeRows: []})} >
                <i><FontAwesomeIcon icon="angle-up"/></i>
              </Button>
            </ToolTip>
        );
    }

    renderFlowStatus = ()=>{
        let flow_status = [
            <ToolTip tooltip={T("View Collection")} key="flow_status">
              <Link role="button"
                    className="btn btn-default btn-outline-info"
                    to={this.viewFlow("overview")}
                >
             <i><FontAwesomeIcon icon="external-link-alt"/></i>
            </Link>
            </ToolTip>
        ];

        if(!_.isEmpty(this.state.flow)) {
            // If the flow is currently running we may be able to stop it.
            if (this.state.flow.state  === 'RUNNING' ||
                this.state.flow.state == "IN_PROGRESS") {
                flow_status.push(
                    <ToolTip tooltip={T("Stop")} key="stop">
                      <Button variant="outline-info shell-info"
                              onClick={this.cancelFlow}>
                        <i><FontAwesomeIcon icon="stop"/></i>
                      </Button>
                      </ToolTip>
                );

                flow_status.push(
                    <Button variant="outline-info shell-info" key="running"
                            disabled>
                      <i><FontAwesomeIcon icon="spinner" spin /></i>
                      <VeloTimestamp usec={this.state.flow.create_time/1000} />
                      - {this.state.flow.request.creator}
                    </Button>
                );

            } else if (this.state.flow.state  === 'FINISHED') {
                flow_status.push(
                    <Button variant="outline-info shell-info" key="finished"
                            disabled>
                      <VeloTimestamp usec={this.state.flow.active_time/1000} />
                      - {this.state.flow.request.creator}
                    </Button>
                );

            } else if (this.state.flow.state  === 'ERROR') {
                flow_status.push(
                    <Button variant="outline-info shell-info" key="error"
                            disabled>
                      <i><FontAwesomeIcon icon="exclamation"/></i>
                      <VeloTimestamp usec={this.state.flow.create_time/1000} />
                      - {this.state.flow.request.creator}
                    </Button>
                );
            }
        }

        flow_status.push(
            <ToolTip tooltip={T("Delete")} key="delete">
              <Button variant="outline-info"
                      disabled={!this.state.flow.session_id}
                      onClick={()=>this.setState({showDeleteWizard: true})}>
                <i><FontAwesomeIcon icon="trash"/></i>
              </Button>
            </ToolTip>
        );

        return flow_status;
    }

    renderButtons =()=>{
        let buttons = [];
        if(!this.props.fullscreen) {
            // Button to load the output from the server (it could be
            // large so we don't fetch it until the user asks)
            buttons.push(this.renderLoadedButtons());
            buttons.push(this.renderExpandAll());

            // The cell can be collapsed ( inside an inset well) or
            // expanded. These buttons can switch between the two
            // modes.
            buttons.push(this.renderCollapsed());

            buttons.push(
                <ToolTip tooltip={T("Full Screen")} key="full screen">
                  <Link to={this.setFullScreen()}
                        target="_blank" rel="noopener noreferrer"
                        role="button" className="btn btn-default">
                    <FontAwesomeIcon icon="maximize"/>
                    <span className="sr-only">{T("Full Screen")}</span>
                  </Link>
                </ToolTip>
            );

        } else {
            buttons.push(<Button variant="default" key="artifact name">
                           { this.props.artifact}
                         </Button>);
            buttons.push(this.renderExpandAll());
        }

        buttons.push(this.transcriptLink());
        return buttons;
    }

    renderLaunchBar = ()=>{
        return <InputGroup className="mb-3 d-flex">
                 <Form.Control as="textarea"
                               rows={1}
                               placeholder={T("Launch a command in this session")}
                               spellCheck="false"
                               value={this.state.command}
                               onChange={e=>this.setState({
                                   command: e.target.value,
                               })} />
                 <Button
                   variant="default"
                   disabled={!this.state.enabled}
                   onClick={this.launchCommand}>
                   {T("Launch")}
                 </Button>
               </InputGroup>;
    }


    // Calculate all the requests from the flow object. This is used
    // to show outstanding requests.
    getAllRequests = ()=>{
        let result = _.map(this.state.flow.previous_flows,
                           flow=>this.getCommand(flow));
        result.push(this.getCommand(this.state.flow));
        return result;
    }

    getCommand = flow=>{
        let res = {
            timestamp: flow.create_time,
        };

        let request = flow.request;
        if(!request || !request.specs) {
            return res;
        }

        res.creator = request.creator;

        let env = [];
        _.each(request.specs, x=>{
            if(x.artifact === this.props.artifact &&
               x.parameters && x.parameters.env) {
                env = x.parameters.env;
            }});

        _.each(env, x=>{
            if(x.key=="Command") {
                res.command = x.value;
            }});

        return res;
    };

    render() {
        let commands = this.getAllRequests();
        let buttons = this.renderButtons();
        let output = "";
        let latest_time = "";
        if (this.state.loaded) {
            let rows = this.state.output || [];

            // Break up the data into a sequence of transactions:
            // command followed by response
            let transactions = [];
            let current_trans = {
                Out: [],
            };
            _.each(rows, (item, idx)=>{
                if(item.Command) {
                    if(current_trans.Command) {
                        // Flush the previous transaction
                        transactions.push(current_trans);
                        current_trans={
                            Out: [],
                        };
                    }
                    current_trans.Command = item.Command;
                    current_trans.Timestamp = item.Timestamp;;
                    latest_time = getTime(current_trans.Timestamp);
                    return;
                };

                if(item.Stdout) {
                    current_trans.Out.push(
                        <span className="stdout" key={"o" + idx}>
                          {item.Stdout}
                        </span>);
                }

                if(item.StdoutUpload) {
                    current_trans.Out.push(
                        <PreviewUpload
                          key={idx}
                          env={{client_id: this.props.client_id,
                                flow_id: this.props.flow_id}}
                          upload={item.StdoutUpload} />);
                }

                if(item.Stderr) {
                    current_trans.Out.push(
                        <span className="stderr" key={"e" + idx}>
                          {item.Stderr}
                        </span>);
                }
            });

            if(current_trans.Command) {
                transactions.push(current_trans);
            }

            let items = _.map(transactions, (item, index) => {
                let command = {};
                if(commands.length > index) {
                    command = commands[index];
                }

                return  <Accordion.Item
                          key={index}
                          eventKey={index} >
                          <Accordion.Header>
                            <Row lg="12">
                              <div className="shell-timestamp-user">
                                <VeloTimestamp
                                  className="float-right"
                                  usec={item.Timestamp} /> ( {command.creator} )
                              </div>
                              <pre>{item.Command}</pre>
                            </Row>
                          </Accordion.Header>
                          <Accordion.Body>
                            <pre>{item.Out}</pre>
                          </Accordion.Body>
                        </Accordion.Item>;
            });

            for(let i=0; i<commands.length;i++) {
                let item = commands[i];
                let ts = parseInt(item.timestamp ) / 1000;
                if(ts < latest_time) {
                    continue;
                }
                items.push(<Accordion.Item key={i} eventKey={i}>
                                         <Accordion.Header>
                                           <Row lg="12">
                                             <div className="shell-timestamp-user">
                                               <VeloTimestamp
                                                 className="float-right"
                                                 usec={item.timestamp} /> ( {item.creator} ) { T("Pending") }
                                             </div>
                                             <pre className="shell-command-pending">
                                               {item.command}
                                             </pre>
                                           </Row>
                                         </Accordion.Header>
                                       </Accordion.Item>);
            }


            output = [
                <Accordion
                  ref={this.scrollRef}
                  key="accordion"
                  activeKey={this.state.activeRows}
                  onSelect={rows=>{
                      this.setState({activeRows: rows});
                  }}
                  alwaysOpen>
                  {items}
                </Accordion>];
        }

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={{client_id: this.props.client_id}}
                  flows={[this.state.flow]}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                  }}
                />
              }
              <div className={classNames({
                  collapsed: this.state.collapsed,
                  expanded: !this.state.collapsed,
                  fullscreen: this.props.fullscreen,
                  'shell-cell': true,
              })}>
                <div className="cell-toolbar">
                  <ButtonToolbar>
                    <ButtonGroup>
                      { buttons }
                    </ButtonGroup>
                    <ButtonGroup className="float-right">
                      { this.renderFlowStatus() }
                    </ButtonGroup>
                  </ButtonToolbar>
                </div>
                <div className="shell-output-container">
                  {output}
                </div>
                { this.state.loaded && this.renderLaunchBar() }
              </div>
            </>
        );
    };
};

const VeloShellCell = withRouter(_VeloShellCell);


class VeloVQLCell extends _VeloShellCell {

    // Retrieve the flow result from the server and show it.
    loadData = (load_now) => {
        if (!this.props.flow_id || !this.props.client_id ||
            !this.props.artifact) {
            return;
        };

        // Get the flow status
        api.get("v1/GetFlowDetails", {
            client_id: this.props.client_id,
            flow_id: this.props.flow_id,
            include_full_request: true,
        }, this.source.token).then((response) => {
            let context = response.data.context;
            if(!_.isEqual(this.state.flow, context)) {
                this.setState({flow: context});
            }
        });

        // Don't fetch anything until the user unfolds the view.
        if (!load_now && !this.state.loaded) {
            return;
        }

        // Get the flow overview first - This is the index of all the
        // VQL queries we sent to this session.
        let artifact = this.props.artifact;
        api.get("v1/GetTable", {
            artifact: artifact + "/Overview",
            client_id: this.props.client_id,
            flow_id: this.props.flow_id,
            rows: 5000,
        }, this.source.token).then(response=>{
            if (!response || !response.data || !response.data.rows) {
                return;
            };
            let data = parseTableResponse(response);

            // Only scroll if the length changes.
            if(this.scrollRef.current &&
               this.state.last_length != data.length) {
                this.scrollToBottom();
            }

            // Do not update the state un necessarily
            if(this.state.last_length != data.length) {
                this.setState({output: data,
                               last_length: data.length,
                               artifact: artifact});
            }
        });
    };

    // The VQL shell does not show a transcript.
    transcriptLink = () => {
        return <span key="transcript"></span>;
    }

    renderLaunchBar = ()=>{
        return <InputGroup className="mb-3 d-flex">
                 <Button
                   variant="default"
                   onClick={this.launchCommand}>
                   {T("Launch")}
                 </Button>

                 <VeloAce
                   mode="vql"
                   className="vql-shell-input"
                   placeholder={T("Run VQL query on client")}
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
               </InputGroup>;
    }

    render() {
        let buttons = this.renderButtons();
        let output = "";
        if (this.state.loaded) {
            let client_id = this.props.client_id;
            let flow_id = this.props.flow_id;
            let artifact = this.props.artifact;
            let rows = this.state.output || [];

            output = [<Accordion
                        ref={this.scrollRef}
                        key="accordion"
                        activeKey={this.state.activeRows}
                        onSelect={rows=>{
                            this.setState({activeRows: rows});
                        }}
                        alwaysOpen>
                        {_.map(rows, (item, index) => {
                            let session_id = item._SessionId;
                            if(!session_id) {
                                return <></>;
                            }
                            return  <Accordion.Item
                                      key={index}
                                      eventKey={index} >
                                      <Accordion.Header>
                                        <Row lg="12">
                                          <VeloTimestamp
                                            className="float-right"
                                            usec={item.Timestamp} />
                                          <Col lg="9">
                                            <pre>{item.Command}</pre>
                                          </Col>
                                          <Col lg="3">
                                          </Col>
                                        </Row>
                                      </Accordion.Header>
                                      <Accordion.Body>
                                        <VeloPagedTable
                                          params={{
                                              artifact: artifact,
                                              flow_id: flow_id,
                                              client_id: client_id,
                                              transform: {
                                                  filter_column: "_SessionId",
                                                  filter_regex: session_id,
                                              },
                                          }}
                                        />
                                        <pre>{item.Out || "" }</pre>
                                      </Accordion.Body>
                                    </Accordion.Item>;
                        })}
                      </Accordion>];
        }

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={{client_id: this.props.client_id}}
                  flows={[this.state.flow]}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                  }}
                />
              }
              <div className={classNames({
                  collapsed: this.state.collapsed,
                  expanded: !this.state.collapsed,
                  fullscreen: this.props.fullscreen,
                  'shell-cell': true,
                  'shell-vql-cell': true,
              })}>
                <div className="cell-toolbar">
                  <ButtonToolbar>
                    <ButtonGroup>
                      { buttons }
                    </ButtonGroup>
                    <ButtonGroup className="float-right">
                      { this.renderFlowStatus() }
                    </ButtonGroup>
                  </ButtonToolbar>
                </div>
                <div className="shell-output-container shell-vql-cell">
                  {output}
                </div>
                { this.state.loaded && this.renderLaunchBar() }
              </div>
            </>
        );
    };
}


class ShellViewer extends Component {
    static propTypes = {
        client_id: PropTypes.string,
        system: PropTypes.string,
        default_shell: PropTypes.string,
    }

    state = {
        flows: [],
        shell_type: "",
        command: "",
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchLastShellCollections,
                                    SHELL_POLL_TIME);
        this.fetchLastShellCollections();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    // Force the flows list to update as soon as the client changes
    // instead of waiting to the next poll.
    componentDidUpdate(prevProps, prevState, snapshot) {
        var new_client_id = this.props.client_id;
        var old_client_id = prevProps.client_id;

        if (new_client_id !== old_client_id) {
            this.fetchLastShellCollections();
        }
    }

    fetchLastShellCollections = () => {
        let client_id = this.props.client_id;
        if (!client_id) {
            return;
        }

        api.get('v1/GetClientFlows', {
            client_id: client_id,
            filter_column: "Artifacts",
            rows: 100,
            start_row: 0,
            sort_direction: false,
            filter_regex: "(Windows.System.PowerShell|Windows.System.CmdShell|Linux.Sys.BashShell|Generic.Client.VQL)"
        }, this.source.token).then(response=>{
            if (response.cancel || !response.data) {
                return;
            }

            let flows = [];
            let rows = response.data.rows || [];
            let columns = response.data.columns || [];
            let column_idx = columns.findIndex(x=>x==="_Flow");
            if (column_idx < 0) {
                console.log("No _Flow column!");
                return;
            }
            for(let i=0; i<rows.length; i++) {
                let row_json = JSONparse(rows[i].json);
                if (!_.isArray(row_json) || row_json.length < column_idx) {
                    continue;
                }

                // Column 8 is the _Flow column;
                let flow = row_json[column_idx];
                if (!flow || !flow.request) {
                    continue;
                }

                var artifacts = flow.request.artifacts;
                for (var j=0; j<artifacts.length; j++) {
                    var artifact = artifacts[j];
                    if (artifact === "Windows.System.PowerShell" ||
                        artifact === "Windows.System.CmdShell" ||
                        artifact === "Generic.Client.VQL" ||
                        artifact === "Linux.Sys.BashShell" ) {
                        flows.push({
                            flow_id: flow.session_id,
                            artifact: artifact,
                        });
                    }
                }
            };

            // Only update if the flows have changed.
            if(!_.isEqual(flows, this.state.flows)) {
                this.setState({flows: flows});
            }
        });
    };

    getArtifactForShell = shell=>{
        switch(shell.toLowerCase()) {
        case "powershell":
            return "Windows.System.PowerShell";

        case "cmd":
            return "Windows.System.CmdShell";

        case "bash" :
            return "Linux.Sys.BashShell";

        case "vql" :
            return "Generic.Client.VQL";
        }
        return "Generic.Client.VQL";
    }

    getShell = ()=>{
        // If the user set the shell selector return that.
        let shell = this.state.shell_type;
        if(shell) {
            return shell;
        }

        // Otherwise find the best match
        shell = this.props.default_shell || 'Powershell';
        if(this.props.system !== "windows") {
            return "Bash";
        }
        return shell;
    }

    launchCommand = () => {
        if (!this.props.client_id) {
            return;
        }

        let shell = this.getShell();
        let artifact = this.getArtifactForShell(shell);

        var params = {
            client_id: this.props.client_id,
            // This is a resumable flow.
            flow_id: "/S",
            artifacts: [artifact],
            specs: [{
                artifact: artifact,
                parameters: {
                    env: [{key: "Command", value: this.state.command}],
                },
                // Prioritise sending the rows quickly
                max_batch_wait: 1,
                max_batch_rows: 1,
            }],
            urgent: true,
        };

        api.post('v1/CollectArtifact', params).then(response=>{
            // Refresh the artifacts immediately.
            this.fetchLastShellCollections();
        }, this.source.token);
    };

    renderCells(flows) {
        return flows.map((item, index) => {
            let artifact = item.artifact;

            if (artifact === "Generic.Client.VQL") {
                return <VeloVQLCell
                         key="vql_cell"
                         fetchLastShellCollections={this.fetchLastShellCollections}
                         flow_id={item.flow_id}
                         artifact={artifact}
                         client_id={this.props.client_id} />;
            };

            return (
                <VeloShellCell key={index}
                               artifact={artifact}
                               fetchLastShellCollections={
                                   this.fetchLastShellCollections}
                               flow_id={item.flow_id}
                               client_id={this.props.client_id} />
            );
        });
    };

    aceConfig = (ace) => {
        // Attach a completer to ACE.
        let completer = new Completer();
        completer.initializeAceEditor(ace, {});
        completer.registerCompletions(this.state.local_completions);

        ace.setOptions({
            autoScrollEditorIntoView: true,
            maxLines: 25,
            placeholder: T("Type VQL to run on the client"),
        });

        this.setState({ace: ace});
    };

    render() {
        let simple_textarea = true;
        let shell_type = this.getShell();
        if (shell_type.toLowerCase() === "vql") {
            simple_textarea = false;
        }

        let client_os = this.props.system;

        return (
            <>
              <Card className="new_session">
                <Card.Body className="shell-command">
                    <InputGroup className="">
                      <DropdownButton as={InputGroup}
                                      title={shell_type}
                                      onSelect={e=>this.setState({shell_type: e})}
                                      id="bg-nested-dropdown">
                        <Dropdown.Item eventKey="Powershell">
                          Powershell
                        </Dropdown.Item>
                        { client_os === "windows" ?
                          <Dropdown.Item eventKey="Cmd">Cmd</Dropdown.Item> :
                          <Dropdown.Item eventKey="Bash">Bash</Dropdown.Item> }
                        <Dropdown.Item eventKey="VQL">VQL</Dropdown.Item>
                      </DropdownButton>
                      { simple_textarea ?
                        <textarea rows="1"
                                  className="form-control"
                                  placeholder={T("Run command in new session")}
                                  spellCheck="false"
                                  value={this.state.command}
                                  onChange={e=>this.setState({
                                      command: e.target.value,
                                  })}>
                        </textarea> :
                        <VeloAce
                          mode="vql"
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
                      <Button disabled={!this.state.command}
                              onClick={this.launchCommand}
                      >{T("Launch")}
                      </Button>
                    </InputGroup>
                </Card.Body>
              </Card>
              <Card className="new_session">
                <Card.Header>{T("Existing sessions")}</Card.Header>
                <Card.Body className="shell-results">
                  { this.renderCells(this.state.flows) }
                </Card.Body>
              </Card>
            </>
        );
    };
}

export default ShellViewer;


class _ShellViewerFullScreen extends Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        // this.fetchLastShellCollections();
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    fetchLastShellCollections = ()=>{}

    state = {
        flow: {},
        client_id: "",
        artifact: "",
    }

    render() {
        let params = (this.props.match && this.props.match.params) || {};
        let client_id = params.client_id;
        let flow_id = params.flow_id;
        let artifact = params.artifact;

        if (artifact === "Generic.Client.VQL") {
            return <VeloVQLCell
                     fetchLastShellCollections={this.fetchLastShellCollections}
                     flow_id={flow_id}
                     artifact={artifact}
                     fullscreen={true}
                     client_id={client_id} />;
        };

        return (
            <VeloShellCell
              flow_id={flow_id}
              artifact={artifact}
              client_id={client_id}
              fullscreen={true}
              fetchLastShellCollections={this.fetchLastShellCollections}
            />
        );
    }
}

export const ShellViewerFullScreen = withRouter(_ShellViewerFullScreen);
