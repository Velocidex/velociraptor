import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";
import VeloTimestamp from "../utils/time.js";
import ShellViewer from "./shell-viewer.js";
import VeloReportViewer from "../artifacts/reporting.js";
import VeloForm from '../forms/form.js';
import { LabelClients } from './clients-list.js';

import ToggleButtonGroup from 'react-bootstrap/ToggleButtonGroup';
import ToggleButton from 'react-bootstrap/ToggleButton';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import Spinner from '../utils/spinner.js';
import Form from 'react-bootstrap/Form';

import { Link } from  "react-router-dom";
import api from '../core/api-service.js';
import axios from 'axios';
import { parseCSV } from '../utils/csv.js';
import "./host-info.css";
import { runArtifact } from "../flows/utils.js";

const POLL_TIME = 5000;
const INTERROGATE_POLL_TIME = 2000;

class QuarantineDialog extends Component {
    static propTypes = {
        client: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        loading: false,
        message: "",
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
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
                    "Windows.Remediation.Quarantine",
                    {MessageBox: this.state.message},
                    ()=>{
                        this.props.onClose();
                        this.setState({loading: false});
                    }, this.source.token);
            });
        }
    }

    render() {
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Quarantine host</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading } />
                <p>You are about to quarantine this host.</p>

                <p>
                  While in quarantine, the host will not be able to
                  communicate with any other networks, except for the
                  Velociraptor server.
                </p>
                <Form>
                  <Form.Group>
                      <Form.Control as="textarea"
                                    placeholder="Quarantine Message"
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
}

class VeloHostInfo extends Component {
    static propTypes = {
        // We must be viewing an actual client.
        client: PropTypes.object,
        setClient: PropTypes.func.isRequired,
    }

    state = {
        // Current inflight interrogate.
        interrogateOperationId: null,

        // The mode of the host info tab set.
        mode: this.props.match.params.action || 'brief',

        metadata: "Key,Value\n,\n",

        loading: false,
        metadata_loading: false,

        showQuarantineDialog: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchMetadata, POLL_TIME);
        this.updateClientInfo();
        this.fetchMetadata();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
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
                    this.setState({loading: false});
                    return this.props.setClient(response.data);
                }, this.source);
        };
    }

    fetchMetadata = () => {
        this.setState({metadata_loading: true});

        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetClientMetadata/" + this.props.client.client_id,
                {}, this.source.token).then(response=>{
                    if (response.cancel) return;

                    let metadata = "Key,Value\n";
                    var rows = 0;
                    var items = response.data["items"] || [];
                    for (var i=0; i<items.length; i++) {
                        var key = items[i]["key"] || "";
                        var value = items[i]["value"] || "";
                        if (!_.isUndefined(key)) {
                            metadata += key + "," + value + "\n";
                            rows += 1;
                        }
                    };
                    if (rows === 0) {
                        metadata = "Key,Value\n,\n";
                    };
                    this.setState({metadata: metadata,
                                   metadata_loading: false});
                });
    }

    setMetadata = (value) => {
        var data = parseCSV(value);
        let items = _.map(data.data, (x) => {
            return {key: x.Key, value: x.Value};
        });

        var params = {client_id: this.props.client.client_id, items: items};
        api.post("v1/SetClientMetadata", params, this.source.token).then(() => {
            this.fetchMetadata();
        });
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

            // Add the quarantine label to this host.
            api.post("v1/LabelClients", {
                client_ids: [client_id],
                operation: "remove",
                labels: ["Quarantine"],
            }, this.source.token).then((response) => {runArtifact(
                client_id,
                "Windows.Remediation.Quarantine",
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
        if (this.state.mode === 'brief') {
            return (
                <CardDeck className="dashboard">
                  <Card>
                    <Card.Header>{ info.os_info.fqdn }</Card.Header>
                    <Card.Body>
                      <dl className="row">
                        <dt className="col-sm-3">Client ID</dt>
                        <dd className="col-sm-9">
                          { info.client_id }
                        </dd>

                        <dt className="col-sm-3">Agent Version</dt>
                        <dd className="col-sm-9">
                          { info.agent_information.version } </dd>

                        <dt className="col-sm-3">Agent Name</dt>
                        <dd className="col-sm-9">
                          { info.agent_information.name } </dd>

                        <dt className="col-sm-3">First Seen At</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={info.first_seen_at * 1000} />
                        </dd>

                        <dt className="col-sm-3">Last Seen At</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={info.last_seen_at / 1000} />
                        </dd>

                        <dt className="col-sm-3">Last Seen IP</dt>
                        <dd className="col-sm-9">
                          { info.last_ip }
                        </dd>

                        <dt className="col-sm-3">Labels</dt>
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
                        <dt className="col-sm-3">Operating System</dt>
                        <dd className="col-sm-9">
                          { info.os_info.system }
                        </dd>

                        <dt className="col-sm-3">Hostname</dt>
                        <dd className="col-sm-9">
                          { info.os_info.hostname }
                        </dd>

                        <dt className="col-sm-3">FQDN</dt>
                        <dd className="col-sm-9">
                          { info.os_info.fqdn }
                        </dd>

                        <dt className="col-sm-3">Release</dt>
                        <dd className="col-sm-9">
                          { info.os_info.release }
                        </dd>

                        <dt className="col-sm-3">Architecture</dt>
                        <dd className="col-sm-9">
                          { info.os_info.machine }
                        </dd>
                      </dl>
                      <hr />
                      <VeloForm
                        param={{type: "csv", name: "Client Metadata"}}
                        value={this.state.metadata}
                        setValue={this.setMetadata}
                      />
                    </Card.Body>
                  </Card>
                </CardDeck>
            );
        };

        if (this.state.mode === 'detailed') {
            return <div className="client-details dashboard">
                     <VeloReportViewer
                       artifact="Custom.Generic.Client.Info"
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

        api.post("v1/CollectArtifact", {
            urgent: true,
            client_id: this.props.client.client_id,
            artifacts: ["Generic.Client.Info"],
        }, this.source.token).then((response) => {
            this.setState({interrogateOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.interrogate_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.interrogateOperationId,
                }, this.source.token).then((response) => {
                    let context = response.data.context;
                    if (context.state === "RUNNING") {
                        return;
                    }

                    // The node is refreshed with the correct flow id, we can stop polling.
                    clearInterval(this.interrogate_interval);
                    this.interrogate_interval = undefined;

                    this.setState({interrogateOperationId: null});
                });
            }, INTERROGATE_POLL_TIME);
        });
    }

    render() {
        let client_id = this.props.client && this.props.client.client_id;
        let info = this.getClientInfo();
        let is_quarantined = info.labels.includes("Quarantine");

        return (
            <>
            { this.state.showQuarantineDialog &&
              <QuarantineDialog client={this.props.client}
                                onClose={()=>this.setState({
                                    showQuarantineDialog: false,
                                })}
              />}
              <div className="full-width-height">
                <div className="client-info">
                  <div className="btn-group float-left toolbar" data-toggle="buttons">
                    <Button variant="default"
                            onClick={this.startInterrogate}
                            disabled={this.state.interrogateOperationId}>
                      { this.state.interrogateOperationId ?
                        <FontAwesomeIcon icon="spinner" spin/>:
                        <FontAwesomeIcon icon="search-plus" /> }
                      <span className="button-label">Interrogate</span>
                    </Button>
                    <Link to={"/vfs/" + client_id + "/"}
                      role="button" className="btn btn-default" >
                      <i><FontAwesomeIcon icon="folder-open"/></i>
                      <span className="button-label">VFS</span>
                    </Link>
                    <Link to={"/collected/" + client_id}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      <span className="button-label">Collected</span>
                    </Link>
                    { is_quarantined ?
                      <Button variant="default"
                              title="Unquarantine Host"
                              onClick={this.unquarantineHost}>
                        <FontAwesomeIcon icon="virus-slash" />
                      </Button> :
                      <Button variant="default"
                              title="Quarantine Host"
                              onClick={()=>this.setState({
                                  showQuarantineDialog: true,
                              })}>
                        <FontAwesomeIcon icon="medkit" />
                    </Button>
                    }
                    { this.state.showLabelDialog &&
                      <LabelClients
                        affectedClients={[{
                            client_id: client_id,
                        }]}
                        onResolve={()=>{
                            this.setState({showLabelDialog: false});
                            this.updateClientInfo();
                        }}/>}
                    <Button variant="default"
                            title="Add Label"
                            onClick={()=>this.setState({
                                showLabelDialog: true,
                            })}>
                        <FontAwesomeIcon icon="tags" />
                    </Button>
                  </div>

                  <ToggleButtonGroup type="radio"
                                     name="mode"
                                     defaultValue={this.state.mode}
                                     onChange={(mode) => this.setMode(mode)}
                                     className="mb-2">
                    <ToggleButton variant="default"
                                  value='brief'>
                      <FontAwesomeIcon icon="laptop"/>
                      <span className="button-label">Overview</span>
                    </ToggleButton>
                    <ToggleButton variant="default"
                                  value='detailed'>
                      <FontAwesomeIcon icon="tasks"/>
                      <span className="button-label">VQL Drilldown</span>
                    </ToggleButton>
                    <ToggleButton variant="default"
                                  value='shell'>
                      <FontAwesomeIcon icon="terminal"/>
                      <span className="button-label">Shell</span>
                    </ToggleButton>
                  </ToggleButtonGroup>
                </div>
                <div className="clearfix"></div>
                { this.renderContent() }
              </div>
            </>
        );
    };
}

export default withRouter(VeloHostInfo);
