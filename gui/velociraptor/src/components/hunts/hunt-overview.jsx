import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import VeloTimestamp from "../utils/time.jsx";
import VeloValueRenderer from "../utils/value.jsx";
import ArtifactLink from '../artifacts/artifacts-link.jsx';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Card from 'react-bootstrap/Card';
import Dropdown from 'react-bootstrap/Dropdown';
import UserConfig from '../core/user.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import T from '../i8n/i8n.jsx';
import ToolTip from '../widgets/tooltip.jsx';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import { requestToParameters } from "../flows/utils.jsx";
import AvailableDownloads from "../notebooks/downloads.jsx";


export default class HuntOverview extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        fetch_hunts: PropTypes.func,
        hunt: PropTypes.object,
    };

    state = {
        preparing: false,
        lock: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();

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
            params.json_format = true;
            params.csv_format = true;
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
                     this.props.fetch_hunts();
        });
    }

    render() {
        let hunt = this.props.hunt;
        if (!hunt) {
            return <div>{T("Please select a hunt to view above.")}</div>;
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
            <Row>
              <Col sm="6">
              <Card>
                <Card.Header>{T("Overview")}</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">{T("Artifact Names")}</dt>
                    <dd className="col-8">
                      { _.map(artifacts, (v, idx) => {
                          return <ArtifactLink
                                   artifact={v}
                                   key={idx}>{v}</ArtifactLink>;
                      })}
                    </dd>

                    <dt className="col-4">{T("Hunt ID")}</dt>
                    <dd className="col-8">{hunt.hunt_id}</dd>

                    <dt className="col-4">{T("Creator")}</dt>
                    <dd className="col-8">{hunt.creator}</dd>

                    <dt className="col-4">{T("Creation Time")}</dt>
                    <dd className="col-8"><VeloTimestamp usec={hunt.create_time / 1000}/></dd>

                    <dt className="col-4">{T("Expiry Time")}</dt>
                    <dd className="col-8"><VeloTimestamp usec={hunt.expires / 1000}/></dd>

                    <dt className="col-4">{T("State")}</dt>
                    <dd className="col-8">{T(this.huntState(hunt))}</dd>

                    <dt className="col-4">{T("Ops/Sec")}</dt>
                    <dd className="col-8">{start_request.ops_per_second ||
                                           T('Unlimited')}</dd>
                    <dt className="col-4">{T("CPU Limit")}</dt>
                    <dd className="col-8">{start_request.cpu_limit ||
                                           T('Unlimited')}</dd>
                    <dt className="col-4">{T("IOPS Limit")}</dt>
                    <dd className="col-8">{start_request.iops_limit ||
                                           T('Unlimited')}</dd>
                    { labels && <>
                                  <dt className="col-4">{T("Include Labels")}</dt>
                                  <dd className="col-8">
                                    {_.map(labels, (v, idx) => {
                                        return <div key={idx}>{v}</div>;
                                    })}
                                  </dd>
                      </>
                    }

                    { hunt.condition && hunt.condition.os &&
                      <>
                        <dt className="col-4">{T("Include OS")}</dt>
                        <dd className="col-8">{hunt.condition.os.os}</dd>
                      </>}
                    { hunt.condition && hunt.condition.excluded_labels &&
                      <>
                        <dt className="col-4">{T("Excluded Labels")}</dt>
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
              </Col>
              <Col sm="6">
              <Card>
                  <Card.Header>{T("Results")}</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">{T("Total scheduled")}</dt>
                    <dd className="col-8">
                      {stats.total_clients_scheduled}
                    </dd>

                    <dt className="col-4">{T("Finished clients")}</dt>
                    <dd className="col-8">{stats.total_clients_with_results || 0}</dd>


                    <dt className="col-4">{T("Download Results")}</dt>
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
                          <ToolTip tooltip={T("Set a password in user preferences to lock the download file.")}>
                            <span className="d-inline-block">
                              <Button
                                style={{ pointerEvents: "none"}}
                                disabled={true}
                                variant="default">
                                <FontAwesomeIcon icon="lock-open"/>
                              </Button>
                            </span>
                          </ToolTip>
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
                                {T("Full Download")}
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary')}>
                              {T("Summary Download")}
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary-csv')}>
                              {T("Summary (CSV Only)")}
                            </Dropdown.Item>
                            <Dropdown.Item
                              onClick={() => this.prepareDownload('summary-json')}>
                              {T("Summary (JSON Only)")}
                            </Dropdown.Item>
                          </Dropdown.Menu>
                        </Dropdown>
                      </ButtonGroup>
                    </dd>
                  </dl>
                  <dl>
                    <dd>
                      <AvailableDownloads
                        files={files}
                      />
                    </dd>
                  </dl>
                </Card.Body>
            </Card>
            </Col>
            </Row>
        );
    }
};
