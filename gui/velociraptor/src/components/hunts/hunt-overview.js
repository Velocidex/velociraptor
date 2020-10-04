import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import VeloTimestamp from "../utils/time.js";
import VeloValueRenderer from "../utils/value.js";

import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import Dropdown from 'react-bootstrap/Dropdown';


import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class HuntOverview extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

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

    render() {
        let hunt = this.props.hunt;
        if (!hunt) {
            return <div>Please select a hunt to view above.</div>;
        };

        let artifacts = hunt.start_request && hunt.start_request.artifacts;
        artifacts = artifacts || [];

        let labels = hunt.condition && hunt.condition.labels && hunt.condition.labels.label;
        let start_request = hunt.start_request || {};
        let parameters = start_request.parameters && start_request.parameters.env;
        parameters = parameters || {};

        let stats = hunt.stats || {};

        return (
            <CardDeck>
              <Card>
                <Card.Header>Overview</Card.Header>
                <Card.Body>
                  <dl className="row">
                    <dt className="col-4">Artifact Names</dt>
                    <dd className="col-8">
                      { _.map(artifacts, (v, idx) => {
                          return <div key={idx}>{v}</div>;
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
                    { labels && <>
                                  <dt>Labels</dt>
                                  <dd>
                                    {_.map(labels, (v, idx) => {
                                        return <div>{v}</div>;
                                    })}
                                  </dd>
                      </>
                    }

                    { hunt.condition && hunt.condition.os &&
                      <>
                        <dt>OS Condition</dt>
                        <dd className="col-8">{hunt.condition.os.os}</dd>
                      </>}
                    <br/>
                  </dl>

                  <h5> Parameters </h5>
                  <dl className="row">
                    { _.map(parameters, (v, idx) => {
                        return <React.Fragment key={idx}>
                          <dt className="col-4">{v.key}</dt>
                          <dd className="col-8"><VeloValueRenderer value={v.value}/></dd>
                        </React.Fragment> ;
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
                      <Dropdown>
                        <Dropdown.Toggle variant="default">
                          <FontAwesomeIcon icon="archive"/>
                        </Dropdown.Toggle>
                        <Dropdown.Menu>
                          <Dropdown.Item
                            onClick={() => this.prepareDownload()}>
                            Full Download
                          </Dropdown.Item>
                          <Dropdown.Item
                            onClick={() => this.prepareReport('summary')}>
                            Summary Download
                          </Dropdown.Item>
                          <Dropdown.Item
                            onClick={() => this.prepareReport('summary-csv')}>
                            Summary (CSV Only)
                          </Dropdown.Item>
                          <Dropdown.Item
                            onClick={() => this.prepareReport('summary-json')}>
                            Summary (JSON Only)
                          </Dropdown.Item>
                        </Dropdown.Menu>
                      </Dropdown>
                    </dd>

                    <dt className="col-4">Available Downloads</dt>
                    <dd>
                      <table className="table table-stiped table-condensed table-hover table-bordered">
                        <tbody>
                          { _.map(stats.available_downloads || [], (item, idx) => {
                              return <tr>
                              <td>
                              { item.complete ? <a href={item.path}>{item.name}</a> :
                                <div>{item.name}</div> }
                              </td><td>{item.size} Bytes</td>
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
