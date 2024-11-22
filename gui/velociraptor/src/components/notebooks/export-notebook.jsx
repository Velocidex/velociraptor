import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import ToolTip from '../widgets/tooltip.jsx';

import Form from 'react-bootstrap/Form';
import FormGroup from 'react-bootstrap/FormGroup';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';
import VeloTimestamp from "../utils/time.jsx";
import VeloTable from "../core/table.jsx";

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const POLL_TIME = 5000;

export default class ExportNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    };

    state = {
        // A detailed notebook record.
        notebook: {},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchNotebookDetails, POLL_TIME);

        // Set the abbreviated notebook in the meantime while we fetch the
        // full details to provide a smoother UX.
        this.setState({notebook: this.props.notebook});
        this.fetchNotebookDetails();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    fetchNotebookDetails = () => {
        api.get("v1/GetNotebooks", {
            include_uploads: true,
            notebook_id: this.props.notebook.notebook_id,
        }, this.source.token).then(resp=>{
            let items = resp.data.items;
            if (_.isEmpty(items)) {
                return;
            }

            this.setState({notebook: items[0]});
        });
    }

    exportNotebook = (type) => {
        api.post("v1/CreateNotebookDownloadFile", {
            notebook_id: this.props.notebook.notebook_id,
            type: type,
        }, this.source.token).then(this.fetchNotebookDetails);
    }

    getDownloadLink = (cell, row) =>{
        var stats = row.stats || {};
        if (row.complete) {
            return <a href={api.href("/api/v1/DownloadVFSFile", {
                fs_components: stats.components,
                vfs_path: row.path,
            })}
                      target="_blank" download
                      rel="noopener noreferrer">{row.name}</a>;
        };
        return <>
                 <FontAwesomeIcon icon="spinner" spin/>
                 <span className="button-label">{row.name}</span>
                 </>;
    }

    render() {
        let files = this.state.notebook && this.state.notebook.available_downloads &&
            this.state.notebook.available_downloads.files;
        files = files || [];

        let columns = ["type", "name", "size", "timestamp"];
        let header_renderers = {
            type: T("Type"),
            name: T("Name"),
            size:  T("Size"),
            timestamp: T("Date"),
        };
        let column_renderers = {
            type:  (cell, row) => {
                if (cell === "html") {
                    return <ToolTip tooltip={T("Zip")}>
                                          <Button disabled={true}
                                                  variant="outline-info">
                                            <FontAwesomeIcon icon="flag"/>
                                          </Button>
                                        </ToolTip>;
                } else if (cell === "zip") {
                    return <ToolTip tooltip={T("HTML")}>
                      <Button disabled={true}
                              variant="outline-info">
                        <FontAwesomeIcon icon="archive"/>
                      </Button>
                    </ToolTip>;
                };
                return cell;
            },
            name: this.getDownloadLink,
            timestamp: (cell, row) => {
                return <VeloTimestamp usec={row.stats.timestamp}/>;
            },
        };

        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.onClose} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Export notebooks")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <FormGroup>
                    <Button variant="default"
                            onClick={()=>this.exportNotebook("html")} >
                      {T("Export to HTML")}
                    </Button>
                    <Button variant="default"
                            onClick={()=>this.exportNotebook("zip")} >
                      {T("Export to Zip")}
                    </Button>
                  </FormGroup>
                </Form>

                <h3>{this.state.notebook.name}</h3>
                {this.state.notebook.description}

                <dt>{T("Available Downloads")}</dt>
                <dd>
                  <VeloTable
                    rows={files}
                    columns={columns}
                    column_renderers={column_renderers}
                    header_renderers={header_renderers}
                  />
                </dd>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.onClose}>
                  {T("Cancel")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
};
