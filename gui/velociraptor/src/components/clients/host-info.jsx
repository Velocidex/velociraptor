import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter, Link }  from "react-router-dom";
import VeloTimestamp from "../utils/time.jsx";
import ShellViewer from "./shell-viewer.jsx";
import VeloReportViewer from "../artifacts/reporting.jsx";
import { LabelClients } from './clients-list.jsx';

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Row from 'react-bootstrap/Row';
import Card from 'react-bootstrap/Card';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Spinner from '../utils/spinner.jsx';
import Form from 'react-bootstrap/Form';
import UserConfig from '../core/user.jsx';
import MetadataEditor from "./metadata.jsx";
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import "./host-info.css";
import { runArtifact } from "../flows/utils.jsx";
import InFlightViewer from "./inflight_viewer.jsx";
import ToolTip from '../widgets/tooltip.jsx';
import classNames from "classnames";

import T from '../i8n/i8n.jsx';

const INTERROGATE_POLL_TIME = 2000;

var quarantine_artifacts = {
    "windows": "Windows.Remediation.Quarantine",
    "linux": "Linux.Remediation.Quarantine",
    "macos": "MacOS.Remediation.Quarantine",
};

class QuarantineDialog extends Component {
    static propTypes = {
        client: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    constructor(props) {
        super(props);
        this.state = {
            loading: false,
            message: "",
            quarantine_available: false,
            quarantine_artifact: quarantine_artifacts[props.client.os_info.system],
        };
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.checkQuarantineAvailability();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    checkQuarantineAvailability() {
        // If the client is running on an OS other than Windows, Linux, or MacOS,
        // they'll need to define an artifact name to use for it
        if (this.state.quarantine_artifact === undefined) {
            this.setState({'quarantine_available' : false});
            return;
        }

        api.post("v1/GetArtifacts",
                 {
                    names: [this.state.quarantine_artifact],
                    number_of_results: 1,
                    // We don't actually need the name, but if we don't specify
                    // a field we get the entire artifact.
                    fields: {
                        name: true
                    },
                 },

                 this.source.token).then((response) => {
                    if (response.cancel) return;

                    let items = response.data.items || [];

                    this.setState({'quarantine_available' : items.length !== 0});
                 });
    }

    startQuarantine = () => {
        let client_id = this.props.client && this.props.client.client_id;

        if (client_id) {
            this.setState({
                loading: true,
            });

            // Add the quarantine label to this host.
            api.post("v1/LabelClients", {
                client_ids: [client_id],
                operation: "set",
                labels: ["Quarantine"],
            }, this.source.token).then((response) => {
                runArtifact(
                    client_id,
                    this.state.quarantine_artifact,
                    {MessageBox: this.state.message},
                    ()=>{
                        this.props.onClose();
                        this.setState({loading: false});
                    }, this.source.token);
            });
        }
    }

    renderAvailable() {
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Quarantine host")}</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading } />
                {T("Quarantine description")}
                <Form>
                  <Form.Group>
                      <Form.Control as="textarea"
                                    placeholder={T("Quarantine Message")}
                                    spellCheck="false"
                                    value={this.state.message}
                                    onChange={e=>this.setState(
                                        {message: e.target.value})}
                        />
                    </Form.Group>
                </Form>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.startQuarantine}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }

    renderUnavailable() {
        let os_name = this.props.client.os_info.system || "an unknown operating system";
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Cannot Quarantine host")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("Cannot Quarantine host message",
                   os_name, this.state.quarantine_artifact)}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="primary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }

    render() {
        if (this.state.quarantine_available) {
            return this.renderAvailable();
        } else {
            return this.renderUnavailable();
        }
    }
}

class VeloHostInfo extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // We must be viewing an actual client.
        client: PropTypes.object,
        setClient: PropTypes.func.isRequired,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    state = {
        // Current inflight interrogate.
        interrogateOperationId: null,

        // The mode of the host info tab set.
        mode: this.props.match.params.action || 'brief',
        loading: false,
        showQuarantineDialog: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.updateClientInfo();
    }

    componentWillUnmount() {
        this.source.cancel();
        if (this.interrogate_interval) {
            clearInterval(this.interrogate_interval);
        }
    }

    // Get the client info object to return something sensible.
    getClientInfo = () => {
        let client_info = this.props.client || {};
        client_info.agent_information = client_info.agent_information || {};
        client_info.os_info = client_info.os_info || {};
        client_info.labels = client_info.labels || [];

        return client_info;
    }

    updateClientInfo = () => {
        this.setState({loading: true});

        let params = {update_mru: true};
        let client_id = this.props.client && this.props.client.client_id;
        if (client_id) {
            api.get("v1/GetClient/" + client_id, params, this.source.token).then(
                response=>{
                    if (!response.data || response.data.client_id !== client_id) {
                        return false;
                    }
                    this.setState({loading: false});
                    return this.props.setClient(response.data);
                }, this.source);
        };
    }

    setMode = (mode) => {
        if (mode !== this.state.mode) {
            let new_state  = Object.assign({}, this.state);
            new_state.mode = mode;
            this.setState(new_state);

            let client_id = this.getClientInfo().client_id;
            if (!client_id) {
                return;
            }

            this.props.history.push('/host/' + client_id + '/' + mode);
        }
    }

    removeLabel = (label) => {
        let client_id = this.getClientInfo().client_id;
        api.post("v1/LabelClients", {
            client_ids: [client_id],
            operation: "remove",
            labels: [label],
        }, this.source.token).then((response) => {
            this.updateClientInfo();
        });
    }

    unquarantineHost = () => {
        let client_id = this.props.client && this.props.client.client_id;

        if (client_id) {
            this.setState({
                loading: true,
            });

        let quarantine_artifact = quarantine_artifacts[this.props.client.os_info.system];

            // Add the quarantine label to this host.
            api.post("v1/LabelClients", {
                client_ids: [client_id],
                operation: "remove",
                labels: ["Quarantine"],
            }, this.source.token).then((response) => {runArtifact(
                client_id,
                quarantine_artifact,
                {RemovePolicy: "Y"},
                ()=>{
                    this.updateClientInfo();
                    this.setState({loading: false});
                }, this.source.token);
            });
        }
    }

    renderContent = () => {
        let info = this.getClientInfo();
        let client_id = this.props.client && this.props.client.client_id;
        if (this.state.mode === 'brief') {
            return (
                <Row className="dashboard">
                  <Card>
                    <Card.Header>{ info.os_info.fqdn }</Card.Header>
                    <Card.Body>
                      <dl className="row">
                        <dt className="col-sm-3">{T("Client ID")}</dt>
                        <dd className="col-sm-9">
                          { info.client_id }
                        </dd>

                        <dt className="col-sm-3">{T("Agent Version")}</dt>
                        <dd className="col-sm-9">
                          { info.agent_information && info.agent_information.version } </dd>

                        <dt className="col-sm-3">{T("Agent Build Time")}</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={
                              info.agent_information && info.agent_information.build_time
                          } />
                        </dd>

                        <dt className="col-sm-3">{T("First Seen At")}</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={info.first_seen_at * 1000} />
                        </dd>

                        <dt className="col-sm-3">{T("Last Seen At")}</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={info.last_seen_at / 1000} />
                        </dd>

                        <dt className="col-sm-3">{T("Last Seen IP")}</dt>
                        <dd className="col-sm-9">
                          { info.last_ip }
                        </dd>

                        <dt className="col-sm-3">{T("Labels")}</dt>
                        <dd className="col-sm-9">
                          { _.map(info.labels, (label, idx) =>{
                              return <Button size="sm" key={idx}
                                             onClick={()=>this.removeLabel(label)}
                                             variant="default">
                                       <span className="button-label">{label}</span>
                                       <span className="button-label">
                                         <FontAwesomeIcon icon="window-close"/>
                                       </span>
                                     </Button>;
                          })}
                        </dd>
                      </dl>
                      <hr />
                      <dl className="row">
                        <dt className="col-sm-3">{T("Operating System")}</dt>
                        <dd className="col-sm-9">
                          { info.os_info.system }
                        </dd>

                        <dt className="col-sm-3">{T("Hostname")}</dt>
                        <dd className="col-sm-9">
                          { info.os_info.hostname }
                        </dd>

                        <dt className="col-sm-3">{T("FQDN")}</dt>
                        <dd className="col-sm-9">
                          { info.os_info.fqdn }
                        </dd>

                        <dt className="col-sm-3">{T("Release")}</dt>
                        <dd className="col-sm-9">
                          { info.os_info.release }
                        </dd>

                        <dt className="col-sm-3">{T("Architecture")}</dt>
                        <dd className="col-sm-9">
                          { info.os_info.machine }
                        </dd>
                        <dt className="col-sm-3">MAC Addresses</dt>
                        <dd className="col-sm-9">
                          { _.map(info.os_info.mac_addresses, (address, idx) => {
                              return <div key={idx}>{address}</div>;
                          })}
                        </dd>
                      </dl>
                      <hr />
                      { info.in_flight_flows &&
                        <>
                          <InFlightViewer client_info={info}/>
                          <hr />
                        </>}
                    </Card.Body>
                    <Card.Header>{T("Client Metadata")}</Card.Header>
                    <Card.Body>
                      <MetadataEditor client_id={client_id} />
                    </Card.Body>
                  </Card>
                </Row>
            );
        };

        if (this.state.mode === 'detailed') {
            return <div className="client-details dashboard">
                     <VeloReportViewer
                       artifact={this.props.client.last_interrogate_artifact_name ||
                                 "Generic.Client.Info"}
                       client={this.props.client}
                       type="CLIENT"
                       flow_id={this.props.client.last_interrogate_flow_id} />
                   </div>;
        }

        if (this.state.mode === 'shell') {
            return (
                <div className="client-details shell">
                  <ShellViewer client={this.props.client} />
                </div>
            );
        }

        return <div>Unknown mode</div>;
    }

    startInterrogate = () => {
        if (this.state.interrogateOperationId) {
            return;
        }
        let interrogate_artifact = "Custom.Generic.Client.Info";

        // 1. Check for custom interrogate artifact
        // 2. Launch the correct interrogate artifact
        // 3. Wait for the flow to complete.
        api.post("v1/GetArtifacts", {
            fields: {name: true},
            name: true,
            number_of_results: 1000,
            search_term: interrogate_artifact,
        }, this.source.token).then((response) => {
            if (_.isEmpty(response.data.items)) {
                interrogate_artifact = "Generic.Client.Info";
            }

            api.post("v1/CollectArtifact", {
                urgent: true,
                client_id: this.props.client.client_id,
                allow_custom_overrides: true,
                artifacts: [interrogate_artifact],
            }, this.source.token).then((response) => {
                this.setState({interrogateOperationId: response.data.flow_id});

                // Start polling for flow completion.
                this.interrogate_interval = setInterval(() => {
                    api.get("v1/GetFlowDetails", {
                        client_id: this.props.client.client_id,
                        flow_id: this.state.interrogateOperationId,
                    }, this.source.token).then((response) => {
                        let context = response.data.context;
                        if (!context || context.state === "RUNNING") {
                            return;
                        }

                        // The node is refreshed with the correct flow id,
                        // we can stop polling.
                        clearInterval(this.interrogate_interval);
                        this.interrogate_interval = undefined;

                        this.setState({interrogateOperationId: null});
                    });
                }, INTERROGATE_POLL_TIME);
            });
        });
    }

    renderQuarantine = ()=>{
        if(this.context.traits &&
           this.context.traits.disable_quarantine_button) {
            return <></>;
        }
        let info = this.getClientInfo();
        let is_quarantined = info.labels.includes("Quarantine");

        if (is_quarantined) {
            return <ToolTip tooltip={T("Unquarantine Host")}>
                     <Button variant="default"
                             onClick={this.unquarantineHost}>
                       <FontAwesomeIcon icon="virus-slash" />
                     </Button>
                   </ToolTip>;
        }
        return <ToolTip tooltip={T("Quarantine Host")}>
                 <Button variant="default"
                         onClick={()=>this.setState({
                             showQuarantineDialog: true,
                         })}>
                   <FontAwesomeIcon icon="medkit" />
                 </Button>
               </ToolTip>;
    }

    render() {
        let client_id = this.props.client && this.props.client.client_id;

        return (
            <>
            { this.state.showQuarantineDialog &&
              <QuarantineDialog client={this.props.client}
                                onClose={()=>this.setState({
                                    showQuarantineDialog: false,
                                })}
              />}
              <div className="full-width-height">
                <Navbar className="toolbar">
                  <ButtonGroup className="" data-toggle="buttons">
                    <Button variant="default"
                            as="a"
                            onClick={e=>{
                                this.startInterrogate();
                                e.preventDefault();
                                return false;
                            }}
                            disabled={this.state.interrogateOperationId}>
                      { this.state.interrogateOperationId ?
                        <FontAwesomeIcon icon="spinner" spin/>:
                        <FontAwesomeIcon icon="search-plus" /> }
                      <span className="button-label">{T("Interrogate")}</span>
                    </Button>
                    <Link to={"/vfs/" + client_id + "/"}
                      role="button" className="btn btn-default" >
                      <i><FontAwesomeIcon icon="folder-open"/></i>
                      <span className="button-label">{T("VFS")}</span>
                    </Link>
                    <Link to={"/collected/" + client_id}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      <span className="button-label">{T("Collected")}</span>
                    </Link>
                    { this.renderQuarantine() }
                    { this.state.showLabelDialog &&
                      <LabelClients
                        affectedClients={[{
                            client_id: client_id,
                        }]}
                        onResolve={()=>{
                            this.setState({showLabelDialog: false});
                            this.updateClientInfo();
                        }}/>}
                    <ToolTip tooltip={T("Add Label")}>
                      <Button variant="default"
                              onClick={e=>{
                                  this.setState({showLabelDialog: true});
                                  e.preventDefault();
                              }}>
                        <FontAwesomeIcon icon="tags" />
                      </Button>
                    </ToolTip>
                  </ButtonGroup>

                  <ButtonGroup className="float-right">
                    <Button variant="default"
                            className={classNames({
                                active: this.state.mode === "brief"})}
                            onClick={(mode) => this.setMode("brief")}>
                      <FontAwesomeIcon icon="laptop"/>
                      <span className="button-label">{T("Overview")}</span>
                    </Button>
                    <Button variant="default"
                            className={classNames({
                                active: this.state.mode === "detailed"})}
                            onClick={(mode) => this.setMode("detailed")}>
                      <FontAwesomeIcon icon="tasks"/>
                      <span className="button-label">{T("VQL Drilldown")}</span>
                    </Button>
                    <Button variant="default"
                            className={classNames({
                                active: this.state.mode === "shell"})}
                            onClick={(mode) => this.setMode("shell")}>
                      <FontAwesomeIcon icon="terminal"/>
                      <span className="button-label">{T("Shell")}</span>
                    </Button>
                  </ButtonGroup>
                </Navbar>
                <div className="clearfix"></div>
                { this.renderContent() }
              </div>
            </>
        );
    };
}

export default withRouter(VeloHostInfo);
