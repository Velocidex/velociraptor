import "./file-stats.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import VeloTimestamp from "../utils/time.js";
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

class VeloFileStats extends Component {
    static propTypes = {
        client: PropTypes.object,
        selectedRow: PropTypes.object,
    }

    state = {
        updateInProgress: false,
    }

    render() {
        if (!this.props.selectedRow || !this.props.selectedRow.Name) {
            return (
                <div className="card">
                  <h5 className="card-header">
                    Click on a file in the table above.
                  </h5>
                </div>
            );
        }

        let selectedRow = Object.assign({
            _FullPath: "",
            Name: "",
            mtime: "",
            atime: "",
            ctime: "",
            Mode: "",
            Size: 0,
            Download: {
                mtime: "",
                vfs_path: "",
                sparse: false,
            },
            _Data: {},
        }, this.props.selectedRow);

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
                      { selectedRow.Download.mtime &&
                        <>
                          <dt className="col-4">
                            Last Collected
                          </dt>
                          <dd className="col-8">
                            <VeloTimestamp usec={ selectedRow.Download.mtime / 1000 } />
                            <Button variant="outline-default" ng-click="controller.downloadFile()"  >
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
                                    ng-click="controller.updateFile()"
                            >
                              <FontAwesomeIcon icon="sync" spin={this.state.updateInProgress} />
                              {selectedRow.Download.vfs_path ? 'Re-Collect' : 'Collect'} from the client
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
                  { selectedRow.Download.sparse &&
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
