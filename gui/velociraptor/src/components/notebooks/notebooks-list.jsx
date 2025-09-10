import "./notebooks-list.css";

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import ExportNotebook from './export-notebook.jsx';
import NotebookUploads from './notebook-uploads.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import VeloPagedTable, {
    TablePaginationControl,
    TransformViewer,
} from '../core/paged-table.jsx';

import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Navbar from 'react-bootstrap/Navbar';
import Modal from 'react-bootstrap/Modal';
import Alert from 'react-bootstrap/Alert';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import { withRouter }  from "react-router-dom";

import T from '../i8n/i8n.jsx';

import { NewNotebook, EditNotebook } from './new-notebook.jsx';
import { getFormatter } from "../core/table.jsx";


class DeleteNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object.isRequired,
        closeDialog: PropTypes.func.isRequired,
        updateNotebooks: PropTypes.func.isRequired,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    deleteNotebook = () => {
        let notebook = Object.assign({}, this.props.notebook);
        notebook.hidden = true;
        api.post("v1/DeleteNotebook", notebook, this.source.token).then(
            this.props.updateNotebooks);
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>
                  {T("Delete notebook")} {this.props.notebook.notebook_id}
                </Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Alert variant="danger" className="text-center">
                  {T("You are about to delete this notebook permanently!")}
                </Alert>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.deleteNotebook}>
                  {T("Do It!!!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class NotebooksList extends React.Component {
    static propTypes = {
        updateVersion: PropTypes.func.isRequired,
        version: PropTypes.number,
        selected_notebook: PropTypes.object,
        setSelectedNotebook: PropTypes.func.isRequired,
        hideToolbar: PropTypes.bool,

        // React router props.
        history: PropTypes.object,
    };

    state = {
        showNewNotebookDialog: false,
        showDeleteNotebookDialog: false,
        showEditNotebookDialog: false,
        showExportNotebookDialog: false,
        showNotebookUploadsDialog: false,

        transform: {},
        filter: "",
        page_state: {},
    }

    setFullScreen = () => {
        if (this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id) {
            this.props.history.push(
                "/fullscreen/notebooks/" +
                    this.props.selected_notebook.notebook_id);
        }
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    render() {
        let column_renderers = {
            "Creation Time": getFormatter("timestamp"),
            "Modified Time": getFormatter("timestamp"),
            "Collaborators": (cell, row) => {
                 return _.map(cell, function(item, idx) {
                     return <div key={idx}>{item}</div>;
                 });
            },
        };

        let selected_notebook = this.props.selected_notebook &&
            this.props.selected_notebook.notebook_id;

        return (
            <>
              { this.state.showDeleteNotebookDialog &&
                <DeleteNotebook
                  notebook={this.props.selected_notebook}
                  updateNotebooks={()=>{
                      this.props.updateVersion();
                      this.setState({showDeleteNotebookDialog: false});
                  }}
                  closeDialog={() => this.setState({showDeleteNotebookDialog: false})}
                />
              }
              { this.state.showNewNotebookDialog &&
                <NewNotebook
                  updateNotebooks={()=>{
                      this.props.updateVersion();
                      this.setState({showNewNotebookDialog: false});
                  }}
                  closeDialog={() => this.setState({showNewNotebookDialog: false})}
                />
              }
              { this.state.showEditNotebookDialog &&
                <EditNotebook
                  notebook={this.props.selected_notebook}
                  updateNotebooks={()=>{
                      this.props.updateVersion();
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

              { this.state.showNotebookUploadsDialog &&
                <NotebookUploads
                  notebook={this.props.selected_notebook}
                  closeDialog={() => this.setState({showNotebookUploadsDialog: false})}
                />
              }

              { !this.props.hideToolbar &&
                <Navbar className="toolbar">
                  <ButtonGroup>
                    <ToolTip tooltip={T("Full Screen")}>
                      <Button disabled={!this.props.selected_notebook ||
                                        !this.props.selected_notebook.notebook_id}
                              onClick={this.setFullScreen}
                              variant="default">
                        <FontAwesomeIcon icon="expand"/>
                        <span className="sr-only">{T("Full Screen")}</span>
                      </Button>
                    </ToolTip>
                    <ToolTip tooltip={T("New Notebook")}>
                      <Button onClick={()=>this.setState({showNewNotebookDialog: true})}
                              variant="default">
                        <FontAwesomeIcon icon="plus"/>
                        <span className="sr-only">{T("New Notebook")}</span>
                      </Button>
                    </ToolTip>
                    <ToolTip tooltip={T("Delete Notebook")}>
                      <Button disabled={_.isEmpty(this.props.selected_notebook)}
                              onClick={()=>this.setState({showDeleteNotebookDialog: true})}
                              variant="default">
                        <FontAwesomeIcon icon="trash"/>
                        <span className="sr-only">{T("Delete Notebook")}</span>
                      </Button>
                    </ToolTip>
                    <ToolTip tooltip={T("Edit Notebook")}>
                      <Button disabled={_.isEmpty(this.props.selected_notebook)}
                              onClick={()=>this.setState({showEditNotebookDialog: true})}
                              variant="default">
                        <FontAwesomeIcon icon="wrench"/>
                        <span className="sr-only">{T("Edit Notebook")}</span>
                      </Button>
                    </ToolTip>
                    <ToolTip tooltip={T("Notebook Uploads")}>
                      <Button disabled={_.isEmpty(this.props.selected_notebook)}
                              onClick={()=>this.setState({showNotebookUploadsDialog: true})}
                              variant="default">
                        <FontAwesomeIcon icon="fa-file-download"/>
                        <span className="sr-only">{T("Notebook Uploads")}</span>
                      </Button>
                    </ToolTip>
                    <ToolTip tooltip={T("Export Notebook")}>
                      <Button disabled={_.isEmpty(this.props.selected_notebook)}
                              onClick={()=>this.setState({showExportNotebookDialog: true})}
                              variant="default">
                        <FontAwesomeIcon icon="download"/>
                        <span className="sr-only">{T("Export Notebook")}</span>
                      </Button>
                    </ToolTip>
                  </ButtonGroup>

                  <ButtonGroup>
                    { this.state.page_state ?
                      <TablePaginationControl
                        total_size={this.state.page_state.total_size}
                        start_row={this.state.page_state.start_row}
                        page_size={this.state.page_state.page_size}
                        current_page={this.state.page_state.start_row /
                                      this.state.page_state.page_size}
                        onRowChange={this.state.page_state.onRowChange}
                        onPageSizeChange={this.state.page_state.onPageSizeChange}
                      /> :  <TablePaginationControl total_size={0}/> }
                  </ButtonGroup>

                  <ButtonGroup>
                    <TransformViewer
                      transform={this.state.transform}
                      setTransform={t=>this.setState({transform: t})}
                    />
                  </ButtonGroup>
                </Navbar>
              }
              <div className="fill-parent no-margins toolbar-margin selectable">
                <VeloPagedTable
                  params={{type: "NOTEBOOKS"}}
                  name="Notebooks"
                  showStackDialog={false}
                  version={{version: this.props.version}}
                  renderers={column_renderers}
                  no_spinner={true}
                  selectRow={{
                      onSelect: (row, idx)=>{
                          this.props.setSelectedNotebook(row.NotebookId);
                      }}}
                  row_classes={row=>{
                      if(row.NotebookId === selected_notebook) {
                          return "row-selected";
                      };
                      return "";
                  }}
                  no_toolbar={true}
                  setPageState={x=>this.setState({page_state: x})}
                  transform={this.state.transform}
                  setTransform={x=>{
                      this.setState({transform: x, filter: ""});
                  }}
                />
              </div>
            </>
        );
    }
};

export default withRouter(NotebooksList);
