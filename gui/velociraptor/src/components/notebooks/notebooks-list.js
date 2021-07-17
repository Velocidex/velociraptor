import "./notebooks-list.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import VeloTimestamp from "../utils/time.js";
import filterFactory from 'react-bootstrap-table2-filter';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import BootstrapTable from 'react-bootstrap-table-next';
import ExportNotebook from './export-notebook.js';

import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Navbar from 'react-bootstrap/Navbar';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Alert from 'react-bootstrap/Alert';

import UserForm from '../utils/users.js';
import api from '../core/api-service.js';

import { formatColumns } from "../core/table.js";
import { withRouter }  from "react-router-dom";


class NewNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
        closeDialog: PropTypes.func.isRequired,
        updateNotebooks: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        if(!_.isEmpty(this.props.notebook)) {
            this.setState({
                name: this.props.notebook.name,
                notebook_id: this.props.notebook.notebook_id,
                description: this.props.notebook.description,
                modified_time: this.props.notebook.modified_time,
                cell_metadata: this.props.notebook.cell_metadata,
                collaborators: this.props.notebook.collaborators || [],
            });
        }
    }

    newNotebook = () => {
        let api_url = "v1/NewNotebook";
        if (!_.isEmpty(this.props.notebook)) {
            api_url = "v1/UpdateNotebook";
        }

        api.post(api_url, {
            name: this.state.name,
            description: this.state.description,
            collaborators: this.state.collaborators,
            modified_time: this.state.modified_time,
            notebook_id: this.state.notebook_id,
            cell_metadata: this.state.cell_metadata,
        }).then(this.props.updateNotebooks);
    }

    state = {
        name: "",
        description: "",
        collaborators: [],
        users: [],
        notebook_id: undefined,
        modified_time: undefined,
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>
                  {_.isEmpty(this.props.notebook) ?
                   "Create a new Notebook" :
                   "Edit notebook " + this.props.notebook.notebook_id}
                </Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Name</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={this.state.name}
                                  onChange={(e) => this.setState(
                                      {name: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">Description</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={this.state.description}
                                  onChange={(e) => this.setState(
                                      {description: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">Collaborators</Form.Label>
                  <Col sm="8">
                    <UserForm
                      value={this.state.collaborators}
                      onChange={(value) => this.setState({collaborators: value})}/>
                  </Col>
                </Form.Group>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.newNotebook}>
                  Submit
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}


class DeleteNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
        updateNotebooks: PropTypes.func.isRequired,
    }

    deleteNotebook = () => {
        let notebook = Object.assign({}, this.props.notebook);
        notebook.hidden = true;
        api.post("v1/UpdateNotebook", notebook).then(
            this.props.updateNotebooks);
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>
                  Archive notebook {this.props.notebook.notebook_id}
                </Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Alert variant="danger" className="text-center">
                  You are about to archive notebook {this.props.notebook.notebook_id}!
                  You can always recover this notebook from the filestore later.
                </Alert>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  Cancel
                </Button>
                <Button variant="primary"
                        onClick={this.deleteNotebook}>
                  Do It!!!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class NotebooksList extends React.Component {
    static propTypes = {
        notebooks: PropTypes.array,
        selected_notebook: PropTypes.object,
        setSelectedNotebook: PropTypes.func.isRequired,
        fetchNotebooks: PropTypes.func.isRequired,
    };

    state = {
        showNewNotebookDialog: false,
        showDeleteNotebookDialog: false,
        showEditNotebookDialog: false,
        showExportNotebookDialog: false,
    }

    setFullScreen = () => {
        if (this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id) {
            this.props.history.push(
                "/fullscreen/notebooks/" +
                    this.props.selected_notebook.notebook_id);
        }
    }

    render() {
        let columns = formatColumns([
            {dataField: "notebook_id", text: "NotebookId"},
            {dataField: "name", text: "Name",
             sort: true, filtered: true },
            {dataField: "description", text: "Description",
             sort: true, filtered: true },
            {dataField: "created_time", text: "Creation Time",
             sort: true, formatter: (cell, row) => {
                return <VeloTimestamp usec={cell * 1000}/>;
            }},
            {dataField: "modified_time", text: "Modified Time",
             sort: true, formatter: (cell, row) => {
                 return <VeloTimestamp usec={cell * 1000}/>;
             }},
            {dataField: "creator", text: "Creator"},
            {dataField: "collaborators", text: "Collaborators",
             sort: true, formatter: (cell, row) => {
                 return _.map(cell, function(item, idx) {
                     return <div key={idx}>{item}</div>;
                 });
             }},
        ]);

        let selected_notebook = this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: function(row) {
                this.props.setSelectedNotebook(row);
            }.bind(this),
            selected: [selected_notebook],
        };

        return (
            <>
              { this.state.showDeleteNotebookDialog &&
                <DeleteNotebook
                  notebook={this.props.selected_notebook}
                  updateNotebooks={()=>{
                      this.props.fetchNotebooks();
                      this.setState({showDeleteNotebookDialog: false});
                  }}
                  closeDialog={() => this.setState({showDeleteNotebookDialog: false})}
                />
              }
              { this.state.showNewNotebookDialog &&
                <NewNotebook
                  updateNotebooks={()=>{
                      this.props.fetchNotebooks();
                      this.setState({showNewNotebookDialog: false});
                  }}
                  closeDialog={() => this.setState({showNewNotebookDialog: false})}
                />
              }
              { this.state.showEditNotebookDialog &&
                <NewNotebook
                  notebook={this.props.selected_notebook}
                  updateNotebooks={()=>{
                      this.props.fetchNotebooks();
                      this.setState({showEditNotebookDialog: false});
                  }}
                  closeDialog={() => this.setState({showEditNotebookDialog: false})}
                />
              }
              { this.state.showExportNotebookDialog &&
                <ExportNotebook
                  notebook={this.props.selected_notebook}
                  onClose={() => this.setState({showExportNotebookDialog: false})}
                />
              }

              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="Full Screen"
                          disabled={!this.props.selected_notebook ||
                                    !this.props.selected_notebook.notebook_id}
                          onClick={this.setFullScreen}
                          variant="default">
                    <FontAwesomeIcon icon="expand"/>
                  </Button>

                  <Button title="NewNotebook"
                          onClick={()=>this.setState({showNewNotebookDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Delete Notebook"
                          onClick={()=>this.setState({showDeleteNotebookDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>

                  <Button title="Edit Notebook"
                          onClick={()=>this.setState({showEditNotebookDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="wrench"/>
                  </Button>
                  <Button title="ExportNotebook"
                          disabled={_.isEmpty(this.props.selected_notebook)}
                          onClick={()=>this.setState({showExportNotebookDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="download"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin selectable">
                {_.isEmpty(this.props.notebooks) ?
                 <div className="no-content">No notebooks available - create one first</div> :
                 <BootstrapTable
                   hover
                   condensed
                   keyField="notebook_id"
                   bootstrap4
                   headerClasses="alert alert-secondary"
                   bodyClasses="fixed-table-body"
                   data={this.props.notebooks}
                   columns={columns}
                   selectRow={ selectRow }
                   filter={ filterFactory() }
                 />}
              </div>
            </>
        );
    }
};

export default withRouter(NotebooksList);
