import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import BootstrapTable from 'react-bootstrap-table-next';
import { formatColumns } from "../core/table.jsx";
import filterFactory from 'react-bootstrap-table2-filter';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import ToolTip from '../widgets/tooltip.jsx';
import classNames from "classnames";
import Form from 'react-bootstrap/Form';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

import T from '../i8n/i8n.jsx';
const POLL_TIME = 5000;

export default class NotebookUploads extends Component {
    static propTypes = {
        notebook: PropTypes.object,
        cell: PropTypes.object,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
        notebook: {},
        upload: {},
        upload_info: {},
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

    uploadFile = () => {
        if (!this.state.upload.name) {
            return;
        }

        let notebook_id = this.props.notebook &&
            this.props.notebook.notebook_id;

        this.setState({loading: true});

        let blob = this.state.upload;
        let reader = new FileReader();
        reader.onload = event=>{
            let request = {
                data: reader.result.split(",")[1],
                notebook_id: notebook_id,
                filename: blob.name,
                size: blob.size,
                disable_attachment_id: true,
            };

            api.post(
                "v1/UploadNotebookAttachment",
                request, this.source.token).then(response => {
                    this.setState({loading:false,
                                   upload: {},
                                   upload_info: {}});
                    this.fetchNotebookDetails();
            }).catch(response=>{
                this.setState({loading: false});
            });
        };
        reader.readAsDataURL(blob);
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

    getDownloadLink = (cell, row) =>{
        let stats = row.stats || {};
        let type = row.type || "";
        let components = stats.components;

        if (!components.length) {
            return <></>;
        };

        components[components.length-1] += type;

        return <a href={
            api.href("/api/v1/DownloadVFSFile", {
                fs_components: components,
                vfs_path: cell + type,
            })}
                  key={row.name}
                  target="_blank" download
                  rel="noopener noreferrer">
                 {row.name} { type && <FontAwesomeIcon icon="note-sticky"/>}
               </a>;
    }

    getDeleteLink = (cell, row) =>{
        let stats = row.stats || {};
        let components = stats.components;

        if (!components.length) {
            return <></>;
        };

        let notebook_id = this.props.notebook &&
            this.props.notebook.notebook_id;

        return <Button key={row.name}
                 onClick={()=>{
            api.post("v1/RemoveNotebookAttachment", {
                components: components,
                notebook_id: notebook_id,
            }, this.source.token).then(response => {
                this.fetchNotebookDetails();
            });
        }}>
                 <FontAwesomeIcon icon="trash"/>
               </Button>;
    }

    render() {
        let files = this.state.notebook &&
            this.state.notebook.available_uploads &&
            this.state.notebook.available_uploads.files;
        files = files || [];

        let columns = formatColumns([
            {dataField: "",
             text: T("Delete"), formatter: this.getDeleteLink},
            {dataField: "name", text: T("Name"),
             sort: true, filtered: true, formatter: this.getDownloadLink},
            {dataField: "size", text: T("Size")},
            {dataField: "date", text: T("Date"), type: "timestamp"},
        ]);

        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>
                  {T("Notebook uploads")}: {this.props.notebook.name}
                </Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Navbar className="toolbar">
                  <ButtonGroup>
                    <ToolTip tooltip={T("Upload")}>
                      <Button
                        disabled={!this.state.upload.name}
                        onClick={this.uploadFile}>
                        { this.state.loading ?
                          <FontAwesomeIcon icon="spinner" spin /> :
                          T("Upload")
                        }
                      </Button>
                    </ToolTip>
                    <Form.Control
                      type="file" id="upload"
                      onChange={e => {
                          if (!_.isEmpty(e.currentTarget.files)) {
                              this.setState({
                                  upload_info: {},
                                  upload: e.currentTarget.files[0],
                              });
                          }
                      }}
                    />
                    { this.state.upload_info.filename &&
                      <a className="btn btn-default-outline"
                         href={ api.href(this.state.upload_info.url) }>
                        { this.state.upload_info.filename }
                      </a>
                    }

                    <ToolTip tooltip={T("Click to upload file")}>
                      <Button variant="default-outline"
                              className="flush-right">
                        <label data-browse={T("Select local file")}
                               htmlFor="upload">
                          {this.state.upload.name ?
                           this.state.upload.name :
                           T("Select local file")}
                        </label>
                      </Button>
                    </ToolTip>

                  </ButtonGroup>
                </Navbar>

                <BootstrapTable
                  hover
                  condensed
                  keyField="path"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={files}
                  columns={columns}
                  filter={ filterFactory() }
                />
                {this.state.showUploadDialog &&
                 <UploadDialog
                   closeDialog={()=>{
                       this.setState({showUploadDialog: false});
                   }}
                   notebook={this.props.notebook}/>}

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
