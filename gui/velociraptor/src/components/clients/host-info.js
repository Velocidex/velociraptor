import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";
import VeloTimestamp from "../utils/time.js";
import ShellViewer from "./shell-viewer.js";
import VeloReportViewer from "../artifacts/reporting.js";
import VeloForm from '../forms/form.js';

import ToggleButtonGroup from 'react-bootstrap/ToggleButtonGroup';
import ToggleButton from 'react-bootstrap/ToggleButton';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import Button from 'react-bootstrap/Button';
import Spinner from '../utils/spinner.js';

import { Link } from  "react-router-dom";
import api from '../core/api-service.js';
import axios from 'axios';
import { parseCSV } from '../utils/csv.js';
import "./host-info.css";

const POLL_TIME = 5000;
const INTERROGATE_POLL_TIME = 2000;

class VeloHostInfo extends Component {
    static propTypes = {
        // We must be viewing an actual client.
        client: PropTypes.object,
    }

    state = {
        // Current inflight interrogate.
        interrogateOperationId: null,

        // The mode of the host info tab set.
        mode: this.props.match.params.action || 'brief',

        metadata: "Key,Value\n,\n",

        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchMetadata, POLL_TIME);
        this.fetchMetadata();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
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

        return client_info;
    }

    fetchMetadata = () => {
        this.setState({loading: true});
        api.get("v1/GetClientMetadata/" + this.props.client.client_id).then(response=>{
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
                this.setState({metadata: metadata, loading: false});
            });
    }

    setMetadata = (value) => {
        var data = parseCSV(value);
        let items = _.map(data.data, (x) => {
            return {key: x.Key, value: x.Value};
        });

        var params = {client_id: this.props.client.client_id, items: items};
        api.post("v1/SetClientMetadata", params).then(() => {
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

    renderContent = () => {
        if (this.state.mode === 'brief') {
            return (
                <CardDeck className="dashboard">
                  <Card>
                    <Card.Header>{ this.getClientInfo().os_info.fqdn }</Card.Header>
                    <Card.Body>
                      <dl className="row">
                        <dt className="col-sm-3">Client ID</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().client_id }
                        </dd>

                        <dt className="col-sm-3">Agent Version</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().agent_information.version } </dd>

                        <dt className="col-sm-3">Agent Name</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().agent_information.name } </dd>

                        <dt className="col-sm-3">Last Seen At</dt>
                        <dd className="col-sm-9">
                          <VeloTimestamp usec={this.getClientInfo().last_seen_at / 1000} />
                        </dd>

                        <dt className="col-sm-3">Last Seen IP</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().last_ip } </dd>
                      </dl>
                      <hr />
                      <dl className="row">
                        <dt className="col-sm-3">Operating System</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().os_info.system }
                        </dd>

                        <dt className="col-sm-3">Hostname</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().os_info.fqdn }
                        </dd>

                        <dt className="col-sm-3">Release</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().os_info.release }
                        </dd>

                        <dt className="col-sm-3">Architecture</dt>
                        <dd className="col-sm-9">
                          { this.getClientInfo().os_info.machine }
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
                     <Spinner loading={this.state.loading} />
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
            client_id: this.props.client.client_id,
            artifacts: ["Generic.Client.Info"],
        }).then((response) => {
            this.setState({interrogateOperationId: response.data.flow_id});

            // Start polling for flow completion.
            this.interrogate_interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: this.state.interrogateOperationId,
                }).then((response) => {
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
        return (
            <>
              <div className="full-width-height">
                <div className="client-info">
                  <div className="btn-group float-left" data-toggle="buttons">
                    <Button variant="default"
                            onClick={this.startInterrogate}
                            disabled={this.state.interrogateOperationId}>
                      { this.state.interrogateOperationId ?
                        <FontAwesomeIcon icon="spinner" spin/>:
                        <FontAwesomeIcon icon="search-plus" /> }
                      <span className="button-label">Interrogate</span>
                    </Button>
                    <Link to={"/vfs/" + this.props.client.client_id + "/"}
                      role="button" className="btn btn-default" >
                      <i><FontAwesomeIcon icon="folder-open"/></i>
                      <span className="button-label">VFS</span>
                    </Link>
                    <Link to={"/collected/" + this.props.client.client_id}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      <span className="button-label">Collected</span>
                    </Link>
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
