import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import VeloTimestamp from "../utils/time.js";
import VeloValueRenderer from "../utils/value.js";
import ArtifactLink from '../artifacts/artifacts-link.js';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import Dropdown from 'react-bootstrap/Dropdown';
import UserConfig from '../core/user.js';
import BootstrapTable from 'react-bootstrap-table-next';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { formatColumns } from "../core/table.js";
import api from '../core/api-service.js';
import axios from 'axios';
import { requestToParameters } from "../flows/utils.js";


export default class HuntOverview extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        hunt: PropTypes.object,
    };

    state = {
        preparing: false,
        lock: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();

        // Default state of the lock is set by the user's preferences.
        let lock_password = this.context.traits &&
            this.context.traits.default_password;
        this.setState({lock: lock_password});
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    huntState = () => {
        let hunt = this.props.hunt;
        let stopped = hunt.stats && hunt.stats.stopped;

        if (stopped || hunt.state === "STOPPED") {
            return "STOPPED";
        }
        if (hunt.state === "RUNNING") {
            return "RUNNING";
        }
        if (hunt.state === "PAUSED") {
            return "PAUSED";
        }
        return "ERROR";
    }

    prepareDownload = (download_type) => {
        let lock_password = "";
        if (this.state.lock) {
            lock_password = this.context.traits &&
                this.context.traits.default_password;
        }

        var params = {
            hunt_id: this.props.hunt.hunt_id,
            password: lock_password,
        };

        switch(download_type) {
        case "all":
            params.json_format = true;
            params.csv_format = true;
            break;

        case 'summary':
            params.only_combined_hunt = true;
            break;

        case 'summary-json':
            params.only_combined_hunt = true;
            params.json_format = true;
            break;

        case 'summary-csv':
            params.only_combined_hunt = true;
            params.csv_format = true;
            break;

        default:
            return;
        }

        this.setState({preparing: true});
        api.post("v1/CreateDownload", params,
                 this.source.token).then(resp=>{
            this.setState({preparing: false});
        });
    }

    render() {
        let hunt = this.props.hunt;
        if (!hunt) {
            return <div>Please select a hunt to view above.</div>;
        };

        let artifacts = hunt.start_request && hunt.start_request.artifacts;
        artifacts = artifacts || [];

        let labels = hunt.condition && hunt.condition.labels && hunt.condition.labels.label;
        let start_request = hunt.start_request || {};
        let parameters = requestToParameters(start_request);
        let lock_password = this.context.traits &&
            this.context.traits.default_password;

        let stats = hunt.stats || {};

        let files = stats.available_downloads && stats.available_downloads.files;
        files = files || [];

        return (
            <CardDeck>
              <Card>
                <Card.Header>Overview</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">Artifact Names</dt>
                    <dd className="col-8">
                      { _.map(artifacts, (v, idx) => {
                          return <ArtifactLink
                                   artifact={v}
                                   key={idx}>{v}</ArtifactLink>;
                      })}
                    </dd>

                    <dt className="col-4">Hunt ID</dt>
                    <dd className="col-8">{hunt.hunt_id}</dd>

                    <dt className="col-4">Creator</dt>
                    <dd className="col-8">{hunt.creator}</dd>

                    <dt className="col-4">Creation Time</dt>
                    <dd className="col-8"><VeloTimestamp usec={hunt.create_time / 1000}/></dd>

                    <dt className="col-4">Expiry Time</dt>
                    <dd className="col-8"><VeloTimestamp usec={hunt.expires / 1000}/></dd>

                    <dt className="col-4">State</dt>
                    <dd className="col-8">{this.huntState(hunt)}</dd>

                    <dt className="col-4">Ops/Sec</dt>
                    <dd className="col-8">{start_request.ops_per_second || 'Unlimited'}</dd>
                    <dt className="col-4">CPU Limit</dt>
                    <dd className="col-8">{start_request.cpu_limit || 'Unlimited'}</dd>
                    <dt className="col-4">IOPS Limit</dt>
                    <dd className="col-8">{start_request.iops_limit || 'Unlimited'}</dd>
                    { labels && <>
                                  <dt className="col-4">Include Labels</dt>
                                  <dd className="col-8">
                                    {_.map(labels, (v, idx) => {
                                        return <div key={idx}>{v}</div>;
                                    })}
                                  </dd>
                      </>
                    }

                    { hunt.condition && hunt.condition.os &&
                      <>
                        <dt className="col-4">Include OS</dt>
                        <dd className="col-8">{hunt.condition.os.os}</dd>
                      </>}
                    { hunt.condition && hunt.condition.excluded_labels &&
                      <>
                        <dt className="col-4">Excluded Labels</dt>
                        <dd className="col-8">
                          {_.map(hunt.condition.excluded_labels.label, (v, idx) => {
                              return <div key={idx}>{v}</div>;
                          })}
                        </dd>
                      </>}

                    <br/>
                  </dl>

                  <h5> Parameters </h5>
                  <dl className="row">
                    {_.map(artifacts, function(name, idx) {
                        return <React.Fragment key={idx}>
                                 <dt className="col-11">{name}</dt><dd className="col-1"/>
                                 {_.map(parameters[name], function(value, key) {
                                     if (value) {
                                         return <React.Fragment key={key}>
                                                  <dt className="col-4">{key}</dt>
                                                  <dd className="col-8">
                                                    <VeloValueRenderer value={value}/>
                                                  </dd>
                                                </React.Fragment>;
                                     };
                                     return <React.Fragment key={key}/>;
                                 })}
                               </React.Fragment>;
                    })}
                  </dl>
                </Card.Body>
              </Card>
              <Card>
                <Card.Header>Results</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">Total scheduled</dt>
                    <dd className="col-8">
                      {stats.total_clients_scheduled}
                    </dd>

                    <dt className="col-4">Finished clients</dt>
                    <dd className="col-8">{stats.total_clients_with_results || 0}</dd>


                    <dt className="col-4">Download Results</dt>
                    <dd className="col-8">
                      <ButtonGroup>
                        { lock_password ?
                          <Button
                            onClick={()=>this.setState({lock: !this.state.lock})}
                            variant="default">
                            {this.state.lock ?
                             <FontAwesomeIcon icon="lock"/> :
                             <FontAwesomeIcon icon="lock-open"/> }
                          </Button>
                          :
                          <OverlayTrigger
                            delay={{show: 250, hide: 400}}
                            overlay={
                                <Tooltip
                                  id='download-tooltip'>
                                  Set a password in user preferences
                                  to lock the download file.
                                </Tooltip>
                            }>
                            <span className="d-inline-block">
                              <Button
                                style={{ pointerEvents: "none"}}
                                disabled={true}
                                variant="default">
                                <FontAwesomeIcon icon="lock-open"/>
                              </Button>
                            </span>
                          </OverlayTrigger>
                        }
                        <Dropdown>
                          <Dropdown.Toggle
                            disabled={this.state.preparing}
                            variant="default">
                            <FontAwesomeIcon icon="archive"/>
                          </Dropdown.Toggle>
                          <Dropdown.Menu>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload("all")}>
                              Full Download
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary')}>
                              Summary Download
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary-csv')}>
                              Summary (CSV Only)
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary-json')}>
                              Summary (JSON Only)
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        </Dropdown>
                      </ButtonGroup>
                    </dd>
                  </dl>
                  <dl>
                    <dt>Available Downloads</dt>
                    <dd>
                      <BootstrapTable
                        keyField="name"
                        condensed
                        bootstrap4
                        hover
                        headerClasses="alert alert-secondary"
                        bodyClasses="fixed-table-body"
                        data={files}
                        columns={formatColumns(
                            [{dataField: "name", text: "name", sort: true,
                              type: "download"},
                             {dataField: "size", text: "size", sort: true},
                             {dataField: "date", text: "date"}])}
                      />
                    </dd>
                  </dl>
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
};
