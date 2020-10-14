import "./file-stats.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import VeloTimestamp from "../utils/time.js";
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import api from '../core/api-service.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import utils from './utils.js';
import axios from 'axios';

const POLL_TIME = 2000;

class VeloFileStats extends Component {
    static propTypes = {
        client: PropTypes.object,
        node: PropTypes.object,
        version: PropTypes.string,
        updateCurrentNode: PropTypes.func,
    }

    state = {
        updateOperation: false,
    }

    updateFile = () => {
        if (this.state.updateInProgress) {
            this.componentWillUnmount();
        }

        let selectedRow = utils.getSelectedRow(this.props.node);
        if (!selectedRow || !selectedRow._FullPath || !selectedRow._Accessor) {
            return;
        }

        let current_mtime = selectedRow && selectedRow.Download &&
            selectedRow.Download.mtime;
        api.post("api/v1/CollectArtifact", {
            client_id: this.props.client.client_id,
            artifacts: ["System.VFS.DownloadFile"],
            parameters: {
                env: [{key: "Path", value: selectedRow._FullPath},
                      {key: "Accessor", value: selectedRow._Accessor}],
            }
        }).then(response => {
            let flow_id = response.data.flow_id;
            this.setState({updateOperation: true});

            // Keep polling until the mtime changes.
            this.source = axios.CancelToken.source();
            this.interval = setInterval(() => {
                api.get("api/v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: flow_id,
                }).then((response) => {
                    if (response.data.context.state === "FINISHED") {
                        this.source.cancel("unmounted");
                        clearInterval(this.interval);
                        this.source = undefined;
                        this.interval = undefined;

                        // Force a tree refresh since this flow is done.
                        let node = this.props.node;
                        node.known = false;
                        node.version = flow_id;
                        this.props.updateCurrentNode(this.props.node);
                        this.setState({updateOperation: false});
                    }
                });
            }, POLL_TIME);
        });
    }

    componentWillUnmount() {
        if (this.source) {
            this.source.cancel("unmounted");
        }
        if (this.interval) {
            clearInterval(this.interval);
        }
    }

    render() {
        let selectedRow = utils.getSelectedRow(this.props.node);
        if (!selectedRow || !selectedRow.Name) {
            return (
                <div className="card">
                  <h5 className="card-header">
                    Click on a file in the table above.
                  </h5>
                </div>
            );
        }

        let client_id = this.props.client && this.props.client.client_id;

        return (
            <CardDeck>
              <Card>
                <Card.Header>{selectedRow._FullPath || selectedRow.Name}</Card.Header>
                <Card.Body>
                    <dl className="row">
                      <dt className="col-4">Size</dt>
                      <dd className="col-8">
                         {selectedRow.Size}
                      </dd>

                      <dt className="col-4">Mode</dt>
                      <dd className="col-8"> {selectedRow.Mode} </dd>

                      <dt className="col-4">Mtime</dt>
                      <dd className="col-8"> {selectedRow.mtime} </dd>

                      <dt className="col-4">Atime</dt>
                      <dd className="col-8"> {selectedRow.atime} </dd>

                      <dt className="col-4">Ctime</dt>
                      <dd className="col-8"> {selectedRow.ctime} </dd>
                      { selectedRow.Download && selectedRow.Download.mtime &&
                        <>
                          <dt className="col-4">
                            Last Collected
                          </dt>
                          <dd className="col-8">
                            <VeloTimestamp usec={ selectedRow.Download.mtime / 1000 } />
                            <Button variant="outline-default"
                                    ng-click="controller.downloadFile()"  >
                              <FontAwesomeIcon icon="download"/>Download
                            </Button>
                          </dd>
                        </>
                      }

                      { selectedRow.Mode[0] === '-' && client_id &&
                        <>
                          <dt className="col-4">Fetch from Client</dt>
                          <dd className="col-8">
                            <Button variant="default"
                                    ng-disabled="!controller.uiTraits.Permissions.collect_client"
                                    onClick={this.updateFile}
                            >
                              <FontAwesomeIcon icon="sync" spin={this.state.updateOperation} />
                              <span className="button-label">
                                {selectedRow.Download && selectedRow.Download.vfs_path ?
                                 'Re-Collect' : 'Collect'} from the client
                              </span>
                            </Button>
                          </dd>
                        </> }
                    </dl>
                </Card.Body>
              </Card>
              <Card>
                <Card.Header>Properties</Card.Header>
                <Card.Body>
                  { _.map(selectedRow._Data, function(v, k) {
                      return <div className="row" key={k}>
                               <dt className="col-4">{k}</dt>
                               <dd className="col-8">{v}</dd>
                             </div>;
                  }) }
                  { selectedRow.Download && selectedRow.Download.sparse &&
                    <div className="row">
                      <dt>Sparse</dt>
                    </div> }
                </Card.Body>
              </Card>
            </CardDeck>
        );
    };
}

export default VeloFileStats;
