import "./file-stats.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

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
            <div className="card-deck">
              <div className="card panel">
                <h5 className="card-header">
                  {selectedRow._FullPath || selectedRow.Name}
                </h5>
                <div className="card-body">
                  <div>
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
                        <div>
                          <dt className="col-4">
                            Last Collected
                          </dt>
                          <dd className="col-8">
                            <grr-timestamp value="controller.fileContext.selectedRow.Download.mtime">
                            </grr-timestamp>
                            <a ng-click="controller.downloadFile()">
                              <i className="fa fa-download"></i>Download
                            </a>
                          </dd>
                        </div>
                      }

                      { selectedRow.Mode[0] == '-' && client_id &&
                        <div>
                          <dt className="col-4">Fetch from Client</dt>
                          <dd className="col-8">
                            <button className="btn btn-default"
                                    ng-disabled="!controller.uiTraits.Permissions.collect_client"
                                    ng-click="controller.updateFile()"
                            >
                              <FontAwesomeIcon icon="refresh" spin={this.state.updateInProgress}/>
                              {selectedRow.Download.vfs_path ? 'Re-Collect' : 'Collect'} from the client
                            </button>
                          </dd>
                        </div> }
                    </dl>
                  </div>
                </div>
              </div>

              <div className="card panel">
                <h5 className="card-header">Properties </h5>
                <div className="card-body">
                  <dl className="row">
                    { _.map(selectedRow._Data, function(k, v) {
                        return <>
                                 <dt className="col-4">{k}</dt>
                                 <dd className="col-8">{v}</dd>
                               </>;
                    }) }
                    { selectedRow.Download.sparse &&
                      <div>
                        <dt>Sparse</dt>
                      </div> }
                  </dl>
                </div>
              </div>
            </div>
        );
    };
}

export default VeloFileStats;
