import "./hunt.css";
import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import BootstrapTable from 'react-bootstrap-table-next';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import filterFactory from 'react-bootstrap-table2-filter';
import cellEditFactory from 'react-bootstrap-table2-editor';
import NotebookUploads from '../notebooks/notebook-uploads.js';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter } from "react-router-dom";
import { formatColumns } from "../core/table.js";

import NewHuntWizard from './new-hunt.js';
import DeleteNotebookDialog from '../notebooks/notebook-delete.js';
import ExportNotebook from '../notebooks/export-notebook.js';
import T from '../i8n/i8n.js';

import api from '../core/api-service.js';
import axios from 'axios';


class HuntList extends React.Component {
    static propTypes = {
        selected_hunt: PropTypes.object,

        // Contain a list of hunt metadata objects - each summary of
        // the hunt.
        hunts: PropTypes.array,
        setSelectedHunt: PropTypes.func,
        updateHunts: PropTypes.func,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();

        let action = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;
        if (action==="new") {
            let name = this.props.match && this.props.match.params &&
                this.props.match.params.tab;

            this.setState({
                showCopyWizard: true,
                full_selected_hunt: {
                    start_request: {
                        artifacts: [name],
                    },
                },
            });
            this.props.history.push("/hunts");
        }
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    state = {
        showWizard: false,
        showRunHuntDialog: false,
        showArchiveHuntDialog: false,
        showDeleteHuntDialog: false,
        showExportNotebook: false,
        showDeleteNotebook: false,
        showCopyWizard: false,
        showNotebookUploadsDialog: false,
    }

    // Launch the hunt.
    setCollectionRequest = (request) => {
        api.post('v1/CreateHunt', request, this.source.token).then((response) => {
            // Keep the wizard up until the server confirms the
            // creation worked.
            this.setState({showWizard: false,
                           showCopyWizard: false});

            // Refresh the hunts list when the creation is done.
            this.props.updateHunts();
        });
    }

    startHunt = () => {
        let hunt_id = this.props.selected_hunt &&
            this.props.selected_hunt.hunt_id;

        if (!hunt_id) {return;};

        api.post("v1/ModifyHunt", {
            state: "RUNNING",
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            this.props.updateHunts();
            this.setState({showRunHuntDialog: false});
        });
    }

    stopHunt = () => {
        let hunt_id = this.props.selected_hunt &&
            this.props.selected_hunt.hunt_id;

        if (!hunt_id) {return;};

        api.post("v1/ModifyHunt", {
            state: "PAUSED",
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            this.props.updateHunts();

            // Start Cancelling all in flight collections in the
            // background.
            api.post("v1/CollectArtifact", {
                client_id: "server",
                artifacts: ["Server.Utils.CancelHunt"],
                specs: [{artifact: "Server.Utils.CancelHunt",
                         parameters: {env: [
                             {key: "HuntId", value: hunt_id},
                         ]}}],
            }, this.source.token);
        });
    }

    archiveHunt = () => {
        let hunt_id = this.props.selected_hunt &&
            this.props.selected_hunt.hunt_id;

        if (!hunt_id) {return;};

        api.post("v1/ModifyHunt", {
            state: "ARCHIVED",
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            this.props.updateHunts();
            this.setState({showArchiveHuntDialog: false});
        });
    }

    updateHunt = (row) => {
        let hunt_id = row.hunt_id;

        if (!hunt_id) {return;};

        api.post("v1/ModifyHunt", {
            hunt_description: row.hunt_description || " ",
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            this.props.updateHunts();
        });
    }

    deleteHunt = () => {
        let hunt_id = this.props.selected_hunt &&
            this.props.selected_hunt.hunt_id;

        if (!hunt_id) {return;};

        // First stop the hunt then delete all the files.
        api.post("v1/ModifyHunt", {
            state: "ARCHIVED",
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            this.props.updateHunts();
            this.setState({showDeleteHuntDialog: false});

            // Start delete collections in the background. It may take
            // a while.
            api.post("v1/CollectArtifact", {
                client_id: "server",
                artifacts: ["Server.Hunts.CancelAndDelete"],
                specs: [{artifact: "Server.Hunts.CancelAndDelete",
                         parameters: {env: [
                             {key: "HuntId", value: hunt_id},
                             {key: "DeleteAllFiles", value: "Y"},
                         ]}}],
            }, this.source.token);
        });
    }

    setFullScreen = () => {
        if (this.props.selected_hunt) {
            this.props.history.push(
                "/fullscreen/hunts/" +
                    this.props.selected_hunt.hunt_id + "/notebook");
        }
    }

    // this.props.selected_hunt is only a hunt summary so we need to
    // fetch the full hunt so we can copy it.
    copyHunt = () => {
        let hunt_id = this.props.selected_hunt &&
            this.props.selected_hunt.hunt_id;

        if (!hunt_id || hunt_id[0] !== "H") {
            return;
        }

        api.get("v1/GetHunt", {
            hunt_id: hunt_id,
        }, this.source.token).then((response) => {
            if (response.cancel) return;

            if(!_.isEmpty(response.data)) {
                this.setState({
                    full_selected_hunt: response.data,
                    showCopyWizard: true,
                });
            }
        });
    }

    render() {
        let columns = getHuntColumns();
        let selected_hunt = this.props.selected_hunt && this.props.selected_hunt.hunt_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            clickToEdit: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: (row) => {
                this.props.setSelectedHunt(row);
            },
            selected: [selected_hunt],
        };

        let state = this.props.selected_hunt && this.props.selected_hunt.state;
        if (this.props.selected_hunt.stats && this.props.selected_hunt.stats.stopped) {
            state = 'STOPPED';
        }

        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        return (
            <>
              { this.state.showWizard &&
                <NewHuntWizard
                  onCancel={(e) => this.setState({showWizard: false})}
                  onResolve={this.setCollectionRequest}
                />
              }
              { this.state.showCopyWizard &&
                <NewHuntWizard
                  baseHunt={this.state.full_selected_hunt}
                  onCancel={(e) => this.setState({showCopyWizard: false})}
                  onResolve={this.setCollectionRequest}
                />
              }

              {this.state.showRunHuntDialog &&
               <Modal show={this.state.showRunHuntDialog}
                      onHide={() => this.setState({showRunHuntDialog: false})} >
                 <Modal.Header closeButton>
                   <Modal.Title>{T("Run this hunt?")}</Modal.Title>
                 </Modal.Header>

                 <Modal.Body>
                   <p>{T("Are you sure you want to run this hunt?")}</p>
                 </Modal.Body>

                 <Modal.Footer>
                   <Button variant="secondary"
                           onClick={() => this.setState({showRunHuntDialog: false})}>
                     {T("Close")}
                   </Button>
                   <Button variant="primary"
                           onClick={this.startHunt}>
                     {T("Run it!")}
                   </Button>
                 </Modal.Footer>
               </Modal>
              }
              { this.state.showDeleteNotebook &&
                <DeleteNotebookDialog
                  notebook_id={"N." + selected_hunt}
                  onClose={e=>{
                      this.setState({showDeleteNotebook: false});
                      this.props.updateHunts();
                  }}/>
              }

              { this.state.showNotebookUploadsDialog &&
                <NotebookUploads
                  notebook={{notebook_id: "N." + selected_hunt}}
                  closeDialog={() => this.setState({showNotebookUploadsDialog: false})}
                />
              }

              { this.state.showExportNotebook &&
                <ExportNotebook
                  notebook={{notebook_id: "N." + selected_hunt}}
                  onClose={(e) => this.setState({showExportNotebook: false})}/>
              }

              { this.state.showDeleteHuntDialog &&
                <Modal show={true}
                       onHide={() => this.setState({showDeleteHuntDialog: false})} >
                  <Modal.Header closeButton>
                    <Modal.Title>{T("Permanently delete this hunt?")}</Modal.Title>
                  </Modal.Header>

                  <Modal.Body>
                    {T("DeleteHuntDialog")}
                 </Modal.Body>

                  <Modal.Footer>
                    <Button variant="secondary"
                            onClick={() => this.setState({showDeleteHuntDialog: false})}>
                      {T("Close")}
                    </Button>
                    <Button variant="primary"
                            onClick={this.deleteHunt}>
                      {T("Kill it!")}
                    </Button>
                  </Modal.Footer>
                </Modal>
              }

              <Navbar className="hunt-toolbar">
                <ButtonGroup>
                  <Button title={T("New Hunt")}
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>
                  <Button title={T("Run Hunt")}
                          disabled={state !== 'PAUSED' && state !== 'STOPPED'}
                          onClick={() => this.setState({showRunHuntDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="play"/>
                  </Button>
                  <Button title={T("Stop Hunt")}
                          disabled={state !== 'RUNNING'}
                          onClick={this.stopHunt}
                          variant="default">
                    <FontAwesomeIcon icon="stop"/>
                  </Button>
                  <Button title={T("Delete Hunt")}
                          disabled={state === 'RUNNING'}
                          onClick={() => this.setState({showDeleteHuntDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="trash-alt"/>
                  </Button>
                  <Button title={T("Copy Hunt")}
                          disabled={!selected_hunt}
                          onClick={this.copyHunt}
                          variant="default">
                    <FontAwesomeIcon icon="copy"/>
                  </Button>
                </ButtonGroup>
                { tab === "notebook" &&
                  <ButtonGroup className="float-right">
                  <Button title={T("Notebooks")}
                            disabled={true}
                            variant="outline-dark">
                      <FontAwesomeIcon icon="book"/>
                    </Button>

                  <Button title={T("Full Screen")}
                            onClick={this.setFullScreen}
                            variant="default">
                      <FontAwesomeIcon icon="expand"/>
                    </Button>

                    <Button title={T("Delete Notebook")}
                            onClick={() => this.setState({showDeleteNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                    </Button>

                    <Button title={T("Notebook Uploads")}
                            onClick={() => this.setState({showNotebookUploadsDialog: true})}
                            variant="default">
                      <FontAwesomeIcon icon="fa-file-download"/>
                    </Button>

                    <Button title={T("Export Notebook")}
                            onClick={() => this.setState({showExportNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="download"/>
                    </Button>
                  </ButtonGroup>
                }

              </Navbar>
              <div className="fill-parent no-margins toolbar-margin selectable">
                <BootstrapTable
                  hover
                  condensed
                  keyField="hunt_id"
                  bootstrap4
                  headerClasses="alert alert-secondary"
                  bodyClasses="fixed-table-body"
                  data={this.props.hunts}
                  columns={columns}
                  filter={ filterFactory() }
                  selectRow={ selectRow }
                  cellEdit={ cellEditFactory({
                      mode: 'dbclick',
                      afterSaveCell: (oldValue, newValue, row, column) => {
                          this.updateHunt(row);
                      },
                      blurToSave: true,
                  }) }
                />
            { _.isEmpty(this.props.hunts) &&
              <div className="no-content">{T("No hunts exist in the system. You can start a new hunt by clicking the New Hunt button above.")}</div>}
              </div>
            </>
        );
    }
};

export default withRouter(HuntList);


export function getHuntColumns() {
    return formatColumns([
        {dataField: "state", text: T("State"),
         formatter: (cell, row) => {
             let stopped = row.stats && row.stats.stopped;

             if (stopped || cell === "STOPPED") {
                 return <div className="hunt-status-icon">
                          <FontAwesomeIcon icon="stop"/></div>;
             }
             if (cell === "RUNNING") {
                 return <div className="hunt-status-icon">
                          <FontAwesomeIcon icon="hourglass"/></div>;
             }
             if (cell === "PAUSED") {
                 return <div className="hunt-status-icon">
                          <FontAwesomeIcon icon="pause"/></div>;
             }
             return <div className="hunt-status-icon">
                          <FontAwesomeIcon icon="exclamation"/></div>;
         }
        },
        {dataField: "hunt_id", text: T("Hunt ID")},
        {dataField: "hunt_description", text: T("Description"),
         sort: true, filtered: true, editable: true},
        {dataField: "create_time", text: T("Created"),
         type: "timestamp", sort: true},
        {dataField: "start_time", text: T("Started"),
         type: "timestamp", sort: true },
        {dataField: "expires",
         text: T("Expires"), sort: true,
         type: "timestamp"},
        {dataField: "stats.total_clients_scheduled", text: T("Scheduled")},
        {dataField: "creator", text: T("Creator")},
    ]);
}
