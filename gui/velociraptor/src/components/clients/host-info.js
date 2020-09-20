import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";
import VeloTimestamp from "../utils/time.js";
import ShellViewer from "./shell-viewer.js";
import VeloReportViewer from "../artifacts/reporting.js";

import ToggleButtonGroup from 'react-bootstrap/ToggleButtonGroup';
import ToggleButton from 'react-bootstrap/ToggleButton';

import { Link } from  "react-router-dom";

import "./host-info.css";


class VeloHostInfo extends Component {
    static propTypes = {
        // We must be viewing an actual client.
        client: PropTypes.object,
    }

    constructor(props) {
        super(props);
        this.state = {
            // Current inflight interrogate.
            interrogateOperationId: null,

            // The mode of the host info tab set.
            mode: this.props.match.params.action || 'brief',
        };

        this.renderInterrogate = this.renderInterrogate.bind(this);
        this.setMode = this.setMode.bind(this);
        this.getClientInfo = this.getClientInfo.bind(this);
    }

    // Get the client info object to return something sensible.
    getClientInfo() {
        let client_info = this.props.client || {};
        client_info.agent_information = client_info.agent_information || {};
        client_info.os_info = client_info.os_info || {};

        return client_info;
    }

    setMode(mode) {
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

    renderInterrogate() {
        if (this.state.interrogateOperationId) {
            return  <FontAwesomeIcon icon="spin" spin/>;
        }
        return <FontAwesomeIcon icon="search-plus" />;
    }

    renderContent() {
        if (this.state.mode === 'brief') {
            return (
                <div className="dashboard">
                  <div className="card panel">
                    <h5 className="card-header">{ this.getClientInfo().os_info.fqdn }</h5>
                    <div className="card-body">
                      <div className="client-details container">
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
                      </div>
                    </div>
                  </div>

                  <div className="card panel">
                    <h5 className="card-header">Metadata</h5>
                    <div className="card-body">
                      <div className="client-details">
                        <grr-csv-form field="'metadata'"
                                      value="controller">
                        </grr-csv-form>
                      </div>
                    </div>
                  </div>
                </div>
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
                <div className="client-details dashboard">
                  <ShellViewer client={this.props.client} />
                </div>
            );
        }

        return <div>Unknown mode</div>;
    }

    render() {
        return (
            <>
              <div className="full-width-height">
                <div className="client-info">
                  <div className="btn-group float-left" data-toggle="buttons">
                    <button className="btn btn-default"
                            ng-click="controller.interrogate()"
                            ng-disabled="controller.interrogateOperationId">
                      { this.renderInterrogate() }
                      Interrogate
                    </button>
                    <Link to={"/vfs/" + this.props.client.client_id}
                      role="button" className="btn btn-default" >
                      <i><FontAwesomeIcon icon="folder-open"/></i>
                      VFS
                    </Link>
                    <Link to={"/collected/" + this.props.client.client_id}
                          role="button" className="btn btn-default">
                      <i><FontAwesomeIcon icon="history"/></i>
                      Collected
                    </Link>
                  </div>

                  <ToggleButtonGroup type="radio"
                                     name="mode"
                                     defaultValue={this.state.mode}
                                     onChange={(mode) => this.setMode(mode)}
                                     className="mb-2">
                    <ToggleButton variant="default"
                                  value='brief'>
                      Overview
                      <i><FontAwesomeIcon icon="laptop"/></i>
                    </ToggleButton>
                    <ToggleButton variant="default"
                                  value='detailed'>
                      VQL Drilldown
                      <i><FontAwesomeIcon icon="tasks"/></i>
                    </ToggleButton>
                    <ToggleButton variant="default"
                                  value='shell'>
                      Shell
                      <i><FontAwesomeIcon icon="terminal"/></i>
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
