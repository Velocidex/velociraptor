import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';

import Form from 'react-bootstrap/Form';
import FormGroup from 'react-bootstrap/FormGroup';
import BootstrapTable from 'react-bootstrap-table-next';
import { formatColumns } from "../core/table.jsx";
import filterFactory from 'react-bootstrap-table2-filter';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import T from '../i8n/i8n.jsx';

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
            }, {internal: true, arrayFormat: 'brackets'})}
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

        let columns = formatColumns([
            {dataField: "type", text: T("Type"), formatter: (cell, row) => {
                if (cell === "html") {
                    return <Button className="btn-tooltip"
                                   disabled={true}
                                   variant="outline-info"
                                   data-tooltip="Zip">
                             <FontAwesomeIcon icon="flag"/>
                           </Button>;
                } else if (cell === "zip") {
                    return <Button className="btn-tooltip"
                                   disabled={true}
                                   variant="outline-info"
                                   data-tooltip="HTML">
                             <FontAwesomeIcon icon="archive"/>
                           </Button>;
                };
                return cell;
            }, sort: true},
            {dataField: "name", text: T("Name"),
             sort: true, filtered: true, formatter: this.getDownloadLink},
            {dataField: "size", text: T("Size")},
            {dataField: "date", text: T("Date"), type: "timestamp"},
        ]);

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
