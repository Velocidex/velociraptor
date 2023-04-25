import "./file-stats.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import VeloTimestamp from "../utils/time.jsx";
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import api from '../core/api-service.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';

const POLL_TIME = 2000;

class VeloFileStats extends Component {
    static propTypes = {
        client: PropTypes.object,

        // The node in the tree for the current directory.
        node: PropTypes.object,

        // The current file selected in the file-list pane.
        selectedRow: PropTypes.object,
        version: PropTypes.string,
        updateCurrentNode: PropTypes.func,
    }

    state = {
        updateOperationFlowId: false,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
        }
    }

    cancelDownload = () => {
        api.post("v1/CancelFlow", {
            client_id: this.props.client.client_id,
            flow_id: this.state.updateOperationFlowId,
        }, this.source.token);
    }

    updateFile = () => {
        if (this.state.updateOperationFlowId) {
            this.source.cancel("unmounted");
            this.source = CancelToken.source();
            if (this.interval) {
                clearInterval(this.interval);
            }
        }

        let selectedRow = this.props.selectedRow;
        if (!selectedRow || !selectedRow._FullPath || !selectedRow._Accessor) {
            return;
        }

        let new_path = [...this.props.node.path];

        // The accessor is the first top level.
        let accessor = new_path.shift();

        // Add the filename to the node.
        new_path.push(selectedRow.Name);

        api.post("v1/CollectArtifact", {
            urgent: true,
            client_id: this.props.client.client_id,
            artifacts: ["System.VFS.DownloadFile"],
            specs: [{artifact: "System.VFS.DownloadFile",
                     parameters: {
                         env: [{key: "Path", value: selectedRow._FullPath},
                               {key: "Components", value: JSON.stringify(new_path)},
                               {key: "Accessor", value: accessor}],
                     }}],
        }, this.source.token).then(response => {
            let flow_id = response.data.flow_id;
            this.setState({updateOperationFlowId: flow_id});

            if(!flow_id) {
                return;
            }

            // Keep polling until the mtime changes.
            this.source = CancelToken.source();
            this.interval = setInterval(() => {
                api.get("v1/GetFlowDetails", {
                    client_id: this.props.client.client_id,
                    flow_id: flow_id,
                }, this.source.token).then((response) => {
                    if (response.data && response.data.context) {
                        if (response.data.context.state !== "RUNNING") {
                            this.source.cancel("unmounted");
                            this.source = CancelToken.source();
                            clearInterval(this.interval);

                            // Force a tree refresh since this flow is done.
                            let node = this.props.node;
                            node.known = false;
                            node.version = flow_id;
                            this.props.updateCurrentNode(this.props.node);
                            this.setState({
                                updateOperationFlowId: undefined,
                                current_upload_bytes: undefined});
                        } else {
                            let uploaded = response.data.context.total_uploaded_bytes || 0;
                            this.setState({
                                current_upload_bytes: uploaded / 1024 / 1024});
                        }
                    }
                });
            }, POLL_TIME);
        });
    }

    render() {
        let selectedRow = this.props.selectedRow;

        if (!selectedRow || !selectedRow.Name) {
            return (
                <div className="card">
                  <h5 className="card-header">
                    {T("Click on a file in the table above.")}
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
                      <dt className="col-4">{T("Size")}</dt>
                      <dd className="col-8">
                         {selectedRow.Size}
                      </dd>

                      <dt className="col-4">{T("Mode")}</dt>
                      <dd className="col-8"> {selectedRow.Mode} </dd>

                      <dt className="col-4">{T("Mtime")}</dt>
                      <dd className="col-8">
                        <VeloTimestamp usec={selectedRow.mtime}/>
                      </dd>

                      <dt className="col-4">{T("Atime")}</dt>
                      <dd className="col-8">
                        <VeloTimestamp usec={selectedRow.atime}/>
                      </dd>

                      <dt className="col-4">{T("Ctime")}</dt>
                      <dd className="col-8">
                        <VeloTimestamp usec={selectedRow.ctime}/>
                      </dd>

                      <dt className="col-4">{T("Btime")}</dt>
                      <dd className="col-8">
                        <VeloTimestamp usec={selectedRow.btime}/>
                      </dd>

                      { selectedRow.Download && selectedRow.Download.mtime &&
                        <>
                          <dt className="col-4">
                            {T("Last Collected")}
                          </dt>
                          <dd className="col-8">
                            <VeloTimestamp usec={ selectedRow.Download.mtime / 1000 } />
                            <Button variant="outline-default"
                                    data-tooltip={T("Download")}
                                    data-position="right"
                                    className="btn-tooltip"
                                    href={api.href("/api/v1/DownloadVFSFile", {
                                        client_id: client_id,
                                        fs_components: selectedRow.Download.components,
                                        vfs_path: selectedRow.Name,
                                    }, {
                                        internal: true,
                                        arrayFormat: 'brackets'})}>
                              <FontAwesomeIcon icon="download"/>
                            </Button>
                          </dd>
                        </>
                      }

                      { selectedRow.Mode[0] === '-' && client_id &&
                        <>
                          <dt className="col-4">{T("Fetch from Client")}</dt>
                          <dd className="col-8">
                        { this.state.updateOperationFlowId ?
                          <Button data-tooltip={T("Currently refreshing from the client")}
                                  data-position="right"
                                  className="btn-tooltip"
                                  onClick={this.cancelDownload}
                                  variant="default">
                            <FontAwesomeIcon icon="sync" spin/>
                            <span className="button-label">{T("Downloaded")} {(
                                this.state.current_upload_bytes || 0) + " Mbytes"}
                            </span>
                            <span className="button-label"><FontAwesomeIcon icon="stop"/></span>
                          </Button> :
                          <Button variant="default"
                                  onClick={this.updateFile}>
                            <FontAwesomeIcon icon="sync" />
                            <span className="button-label">
                              {selectedRow.Download &&
                               !_.isEmpty(selectedRow.Download.components) ?
                               T('Re-Collect from the client') :
                               T('Collect from the client')}
                            </span>
                          </Button> }
                          </dd>
                        </> }
                    </dl>
                </Card.Body>
              </Card>
              <Card>
                <Card.Header>{T("Properties")}</Card.Header>
                <Card.Body>
                  { _.map(selectedRow._Data, function(v, k) {
                      return <div className="row" key={k}>
                               <dt className="col-4">{k}</dt>
                               <dd className="col-8">{v}</dd>
                             </div>;
                  }) }
                  { selectedRow.Download && selectedRow.Download.sparse &&
                    <div className="row">
                      <dt>{T("Sparse")}</dt>
                    </div> }
                  { selectedRow.Download && selectedRow.Download.SHA256 &&
                    <div className="row">
                      <dt className="col-4">SHA256</dt>
                      <dd className="col-8">{selectedRow.Download.SHA256}</dd>
                    </div>}
                  { selectedRow.Download && selectedRow.Download.MD5 &&
                    <div className="row">
                      <dt className="col-4">MD5</dt>
                      <dd className="col-8">{selectedRow.Download.MD5}</dd>
                    </div>}
                  { !_.isEmpty(selectedRow.Download.components) &&
                    <div className="row">
                      <dt className="col-4">{T("Preview")}</dt>
                      <dd className="col-8">
                        <PreviewUpload
                          env={{client_id: this.props.client.client_id,
                                vfs_components: selectedRow.Download.components}}
                          upload={{Components: selectedRow.Download.components,
                                   Path: selectedRow._FullPath,
                                   Accessor: this.props.node.path[0],
                                   Size: selectedRow.Size}} />
                      </dd>
                    </div>}
                </Card.Body>
              </Card>
            </CardDeck>
        );
    };
}

export default VeloFileStats;
