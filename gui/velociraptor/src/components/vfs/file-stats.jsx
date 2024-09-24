import "./file-stats.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import VeloTimestamp from "../utils/time.jsx";
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Card from 'react-bootstrap/Card';
import api from '../core/api-service.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import UserConfig from '../core/user.jsx';

class VeloFileStats extends Component {
    static contextType = UserConfig;

    static propTypes = {
        client: PropTypes.object,

        // The node in the tree for the current directory.
        node: PropTypes.object,
        bumpVersion: PropTypes.func,

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
        let selectedRow = this.props.selectedRow;
        if (!selectedRow || !(selectedRow._OSPath || selectedRow._FullPath) || !selectedRow._Accessor) {
            return;
        }

        this.source.cancel("unmounted");
        this.source = CancelToken.source();

        let new_path = [...this.props.node.path];

        // The accessor is the first top level.
        let accessor = new_path.shift();

        // Add the filename to the node.
        new_path.push(selectedRow.Name);

        api.post("v1/VFSDownloadFile", {
            client_id: this.props.client.client_id,
            accessor: accessor,
            components: new_path,
        }, this.source.token).then(response => {
            let flow_id = response.data.flow_id;
            this.setState({updateOperationFlowId: flow_id});

            if(!flow_id) {
                return;
            }
            this.props.bumpVersion();
        });
    }

    render() {
        let selectedRow = this.props.selectedRow;
        let inflight = selectedRow && selectedRow.Download &&
            selectedRow.Download.in_flight;

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
        let no_password = !this.context.traits.default_password;

        return (
            <Row className="file-stats">
              <Col sm="6">
              <Card>
                <Card.Header>{selectedRow._OSPath || selectedRow._FullPath || selectedRow.Name}</Card.Header>
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
                            { !_.isEmpty(selectedRow.Download.components) &&
                              <ToolTip tooltip={T("Download")}>
                                <Button variant="default"
                                        href={api.href("/api/v1/DownloadVFSFile", {
                                            client_id: client_id,
                                            fs_components: selectedRow.Download.components,
                                            vfs_path: selectedRow.Name,
                                            zip: !no_password,
                                        })}>
                                  { !no_password &&
                                    <div className="velo-icon">
                                      <FontAwesomeIcon icon="lock" />
                                    </div>}
                                  <div className="velo-icon">
                                    <FontAwesomeIcon icon="download"/>
                                  </div>
                                  <VeloTimestamp usec={
                                      selectedRow.Download.mtime / 1000
                                  } />
                                </Button>
                              </ToolTip>
                            }
                          </dd>
                        </>
                      }

                      { selectedRow.Mode[0] === '-' && client_id &&
                        <>
                          <dt className="col-4">{T("Fetch from Client")}</dt>
                          <dd className="col-8">
                            <Button variant="default"
                                    onClick={this.updateFile}>
                              <FontAwesomeIcon icon="sync" />
                              <span className="button-label">
                                {selectedRow.Download &&
                                 !_.isEmpty(selectedRow.Download.components) ?
                                 T('Re-Collect from the client') :
                                 T('Collect from the client')}
                              </span>
                            </Button>
                          </dd>
                        </> }
                    </dl>
                </Card.Body>
            </Card>
              </Col>
            <Col sm="6">
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
                  { selectedRow.Download && selectedRow.Download.error &&
                    <div className="row">
                      <dt className="col-4">{T("Download Error")}</dt>
                      <dd className="col-8">{selectedRow.Download.error}</dd>
                    </div>}
                  { !inflight && !_.isEmpty(selectedRow.Download.components) &&
                    <div className="row">
                      <dt className="col-4">{T("Preview")}</dt>
                      <dd className="col-8">
                        <PreviewUpload
                          env={{client_id: this.props.client.client_id,
                                vfs_components: selectedRow.Download.components}}
                          upload={{Components: selectedRow.Download.components,
                                   Path: selectedRow._OSPath || selectedRow._FullPath,
                                   Accessor: this.props.node.path[0],
                                   Size: selectedRow.Size}} />
                      </dd>
                    </div>}
                </Card.Body>
            </Card>
            </Col>
            </Row>
        );
    };
}

export default VeloFileStats;
