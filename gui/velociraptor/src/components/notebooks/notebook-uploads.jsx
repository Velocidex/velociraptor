import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import BootstrapTable from 'react-bootstrap-table-next';
import { formatColumns } from "../core/table.jsx";
import filterFactory from 'react-bootstrap-table2-filter';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

import T from '../i8n/i8n.jsx';
const POLL_TIME = 5000;

export default class NotebookUploads extends Component {
    static propTypes = {
        notebook: PropTypes.object,
        closeDialog: PropTypes.func.isRequired,
    }

    state = {
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

    getDownloadLink = (cell, row) =>{
        let stats = row.stats || {};
        let type = row.type || "";
        let components = stats.components;

        if (!components.length) {
            return <></>;
        };

        components[components.length-1] += type;

        return <a href={api.href("/api/v1/DownloadVFSFile", {
            fs_components: components,
            vfs_path: cell + type,
        }, {internal: true, arrayFormat: 'brackets'})}
                  target="_blank" download
                  rel="noopener noreferrer">
                 {row.name} { type && <FontAwesomeIcon icon="note-sticky"/>}
               </a>;
    }

    getDeleteLink = (cell, row) =>{
        var stats = row.stats || {};
        return <Button onClick={()=>{
            api.post("v1/RemoveNotebookAttachment", {
                components: stats.components,
                notebook_id: this.props.notebook.notebook_id,
            }, this.source.token).then(this.fetchNotebookDetails);
        }}><FontAwesomeIcon icon="trash"/> </Button>;
    }

    render() {
        let files = this.state.notebook &&
            this.state.notebook.available_uploads &&
            this.state.notebook.available_uploads.files;
        files = files || [];

        let columns = formatColumns([
            {dataField: "name_", text: "", formatter: this.getDeleteLink},
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
