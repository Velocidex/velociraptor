import React, { Fragment } from 'react';
import PropTypes from 'prop-types';
import VeloTimestamp from "../utils/time.js";
import VeloValueRenderer from "../utils/value.js";

import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import Dropdown from 'react-bootstrap/Dropdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import _ from 'lodash';

import api from '../core/api-service.js';

import axios from 'axios';

const POLL_TIME = 5000;

export default class FlowOverview extends React.Component {
    static propTypes = {
        // This is the abbreviated flow details. When the component
        // mounts we start polling for the detailed flow details and
        // replace it.
        flow: PropTypes.object,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchDetailedFlow, POLL_TIME);

        // Set the abbreviated flow in the meantime while we fetch the
        // full detailed to provide a smoother UX.
        this.setState({detailed_flow: {context: this.props.flow}});
        this.fetchDetailedFlow();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        if (this.props.flow.session_id !== prev_flow_id) {
            this.setState({detailed_flow: {context: this.props.flow}});
            this.fetchDetailedFlow();
        };
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    fetchDetailedFlow = () => {
        if (!this.props.flow || !this.props.flow.client_id) {
            return;
        };

        api.get("v1/GetFlowDetails", {
            flow_id: this.props.flow.session_id,
            client_id: this.props.flow.client_id,
        }).then((response) => {
            this.setState({detailed_flow: response.data});
        });
    }

    prepareDownload = () => {

    };

    prepareReport = () => {

    };

    state = {
        detailed_flow: {},
    }

    render() {
        let flow = this.state.detailed_flow && this.state.detailed_flow.context;
        let artifacts = flow && flow.request && flow.request.artifacts;

        if (!flow || !flow.session_id || !artifacts)  {
            return <h5 className="no-content">Please click a collection in the above table</h5>;
        }

        let parameters = flow.request &&
            flow.request.parameters && flow.request.parameters.env;
        parameters = parameters || [];

        let artifacts_with_results = flow.artifacts_with_results || [];
        let uploaded_files = flow.uploaded_files || [];
        let available_downloads = this.state.detailed_flow &&
            this.state.detailed_flow.available_downloads &&
            this.state.detailed_flow.available_downloads.files;
        available_downloads = available_downloads || [];

        return (
            <CardDeck>
              <Card>
                <Card.Header>Overview</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">Artifact Names</dt>
                    <dd className="col-8">
                      { _.map(artifacts, function(v, idx) {
                          return <div key={idx}>{v}</div>;
                      })}
                    </dd>

                    <dt className="col-4">Flow ID</dt>
                    <dd className="col-8">  { flow.session_id } </dd>

                    <dt className="col-4">Creator</dt>
                    <dd className="col-8"> { flow.request.creator } </dd>

                    <dt className="col-4">Create Time</dt>
                    <dd className="col-8">
                      <VeloTimestamp usec={flow.create_time / 1000}/>
                    </dd>

                    <dt className="col-4">Start Time</dt>
                    <dd className="col-8">
                      <VeloTimestamp usec={flow.start_time / 1000}/>
                    </dd>

                    <dt className="col-4">Last Active</dt>
                    <dd className="col-8">
                      <VeloTimestamp usec={flow.active_time / 1000}/>
                    </dd>

                    <dt className="col-4">Duration</dt>
                    <dd className="col-8"> {
                        flow.execution_duration ?
                            ((flow.execution_duration)/1000000000).toFixed(2) : ""
                    }
                      { flow.execution_duration ? " Seconds" : " Running..."}
                    </dd>

                    <dt className="col-4">State</dt>
                    <dd className="col-8">{ flow.state }</dd>

                    { flow.state === "ERROR" &&
                      <Fragment>
                        <dt className="col-4">Error</dt>
                        <dd className="col-8">{flow.status }</dd>
                      </Fragment>
                    }

                    <dt className="col-4">Ops/Sec</dt>
                    <dd className="col-8"> {flow.request.ops_per_second || 'Unlimited'} </dd>
                    <dt className="col-4">Timeout</dt>
                    <dd className="col-8"> {flow.request.timeout || 'Unlimited' } </dd>
                    <dt className="col-4">Max Rows</dt>
                    <dd className="col-8"> {flow.request.max_rows || 'Unlimited'} </dd>
                    <dt className="col-4">Max Mb</dt>
                    <dd className="col-8"> { ((flow.request.max_upload_bytes || 0)
                                              / 1024 / 1024).toFixed(2) || 'Unlimited' }</dd>
                    <br />
                  </dl>

                  <h5> Parameters </h5>
                  <dl className="row">
                    { _.map(parameters, function(item, idx) {
                        if (item) {
                            return <React.Fragment key={idx}>
                                     <dt className="col-4">{item.key}</dt>
                                     <dd className="col-8">
                                       <VeloValueRenderer value={item.value}/>
                                     </dd>
                                   </React.Fragment>;
                        };
                        return <>{JSON.stringify(item)}</>;
                    })}
                  </dl>
                </Card.Body>
              </Card>
              <Card>
                <Card.Header>Results</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">Artifacts with Results</dt>
                    <dd className="col-8">
                      { _.map(artifacts_with_results, function(item, idx) {
                          return <VeloValueRenderer value={item} key={idx}/>;
                      })}
                    </dd>

                    <dt className="col-4">Total Rows</dt>
                    <dd className="col-8">
                      { flow.total_collected_rows || 0 }
                    </dd>

                    <dt className="col-4">Uploaded Bytes</dt>
                    <dd className="col-8">
                      { (flow.total_uploaded_bytes || 0) } / {
                        (flow.total_expected_uploaded_bytes || 0) }
                    </dd>

                    <dt className="col-4">Files uploaded</dt>
                    <dd className="col-8">
                      {uploaded_files.length || flow.total_uploaded_files || 0 }
                    </dd>

                    <dt className="col-4">Download Results</dt>
                    <dd className="col-8">
                      <Dropdown>
                        <Dropdown.Toggle variant="default">
                          <FontAwesomeIcon icon="archive"/>
                        </Dropdown.Toggle>
                        <Dropdown.Menu>
                          <Dropdown.Item
                            onClick={this.prepareDownload}>
                            Prepare Download
                          </Dropdown.Item>
                          <Dropdown.Item
                            onClick={this.prepareReport}>
                            Prepare Collection Report
                          </Dropdown.Item>
                        </Dropdown.Menu>
                      </Dropdown>
                    </dd>

                    <dt className="col-4">Available Downloads</dt>
                    <dd className="col-8">
                      <table className="table table-stiped table-condensed table-hover table-bordered">
                        <tbody>
                          { _.map(available_downloads, function(item, idx) {
                              return <tr key={idx}>
                                       <td>
                                         { item.complete ?
                                           <a href={item.path}
                                              rel="noopener noreferrer"
                                              target="_blank">{item.name}</a> :
                                           <div>{item.name}</div>
                                         }
                                       </td>
                                       <td>{item.size} Bytes</td>
                                       <td>{item.date}</td>
                                     </tr>;
                          })}
                        </tbody>
                      </table>
                    </dd>
                  </dl>
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
};
