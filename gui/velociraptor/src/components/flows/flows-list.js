
import "./flows.css";

import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';

import api from '../core/api-service.js';
import { formatColumns } from "../core/table.js";

import NewCollectionWizard from './new-collection.js';
import OfflineCollectorWizard from './offline-collector.js';
import Spinner from '../utils/spinner.js';
import DeleteNotebookDialog from '../notebooks/notebook-delete.js';
import ExportNotebook from '../notebooks/export-notebook.js';

import { NavLink } from "react-router-dom";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HotKeys } from "react-hotkeys";
import { withRouter } from "react-router-dom";
import { runArtifact } from "./utils.js";

import Modal from 'react-bootstrap/Modal';
import UserConfig from '../core/user.js';
import VeloForm from '../forms/form.js';
import AddFlowToHuntDialog from './flows-add-to-hunt.js';

import axios from 'axios';


export class DeleteFlowDialog extends React.PureComponent {
    static propTypes = {
        client: PropTypes.object,
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    startDeleteFlow = () => {
        let client_id = this.props.client && this.props.client.client_id;
        let flow_id = this.props.flow && this.props.flow.session_id;

        if (flow_id && client_id) {
            this.setState({loading: true});
            runArtifact("server",   // This collection happens on the server.
                        "Server.Utils.DeleteFlow",
                        {FlowId: flow_id,
                         ClientId: client_id,
                         ReallyDoIt: "Y"}, ()=>{
                             this.props.onClose();
                             this.setState({loading: false});
                         }, this.source.token);
        }
    }

    render() {
        let collected_artifacts = this.props.flow.artifacts_with_results || [];
        let artifacts = collected_artifacts.join(",");
        let total_bytes = this.props.flow.total_uploaded_bytes/1024/1024 || 0;
        let total_rows = this.props.flow.total_collected_rows || 0;
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Permanently delete collection</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading} />
                You are about to permanently delete the artifact collection
                <b>{this.props.flow.session_id}</b>.
                <br/>
                This collection was the artifacts <b>{artifacts}</b>
                <br/><br/>

                We expect to free up { total_bytes.toFixed(0) } Mb of bulk
                data and { total_rows } rows.
                </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.startDeleteFlow}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

export class SaveCollectionDialog extends React.PureComponent {
    static contextType = UserConfig;
    static propTypes = {
        client: PropTypes.object,
        flow: PropTypes.object,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
     }

    componentWillUnmount() {
        this.source.cancel();
    }

    startSaveFlow = () => {
        let client_id = this.props.client && this.props.client.client_id;
        let specs = this.props.flow.request.specs;
        let type = "CLIENT";
        if (client_id==="server") {
            type = "SERVER";
        }

        this.setState({loading: true});
        runArtifact("server",   // This collection happens on the server.
                    "Server.Utils.SaveFavoriteFlow",
                    {
                        Specs: JSON.stringify(specs),
                        Name: this.state.name,
                        Description: this.state.description,
                        Type: type,
                    }, ()=>{
                        this.props.onClose();
                        this.setState({loading: false});
                    }, this.source.token);
    }

    render() {
        let collected_artifacts = this.props.flow.artifacts_with_results || [];
        let artifacts = collected_artifacts.join(",");
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Save this collection to your Favorites</Modal.Title>
              </Modal.Header>
              <Modal.Body><Spinner loading={this.state.loading} />
                You can easily collect the same collection from your
                favorites in future.
                <br/>
                This collection was the artifacts <b>{artifacts}</b>
                <br/><br/>
                <VeloForm
                  param={{name: "Name", description: "New Favorite name"}}
                  value={this.state.name}
                  setValue={x=>this.setState({name:x})}
                />
                <VeloForm
                  param={{name: "Description",
                          description: "Describe this favorite"}}
                  value={this.state.description}
                  setValue={x=>this.setState({description:x})}
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.startSaveFlow}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class FlowsList extends React.Component {
    static propTypes = {
        client: PropTypes.object,
        flows: PropTypes.array,
        setSelectedFlow: PropTypes.func,
        selected_flow: PropTypes.object,
        fetchFlows: PropTypes.func,
    };

    state = {
        showWizard: false,
        showAddToHunt: false,
        showCopyWizard: false,
        showOfflineWizard: false,
        showDeleteWizard: false,
        showDeleteNotebook: false,
        initialized_from_parent: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
        }
        if (this.recrusive_interval) {
            clearInterval(this.recrusive_interval);
        }
    }

    // Set the table in focus when the component mounts for the first time.
    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        if (!this.state.initialized_from_parent && selected_flow) {
            const el = document.getElementById(selected_flow);
            if (el) {
                this.setState({initialized_from_parent: true});
                el.focus();
            }
        }
    }

    setCollectionRequest = (request) => {
        // Make a request to start the flow on this client.
        request.client_id = this.props.client.client_id;
        api.post("v1/CollectArtifact", request).then((response) => {
            // When the request is done force our parent to refresh.
            this.props.fetchFlows();

            // Only disable wizards if the request was successful.
            this.setState({showWizard: false,
                           showOfflineWizard: false,
                           showCopyWizard: false});
        });
    }

    cancelButtonClicked = () => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let flow_id = this.props.selected_flow && this.props.selected_flow.session_id;

        if (client_id && flow_id) {
            api.post("v1/CancelFlow", {
                client_id: client_id, flow_id: flow_id
            }).then((response) => {
                this.props.fetchFlows();
            });
        }
    }

    // Navigates to the next row to the one that is highlighted
    gotoNextRow = () => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        for(let i=0; i<this.node.table.props.data.length; i++) {
            let row = this.node.table.props.data[i];
            if (row.session_id === selected_flow) {
                // Last row
                if (i+1 >= this.node.table.props.data.length) {
                    return;
                }
                let next_row = this.node.table.props.data[i+1];
                this.props.setSelectedFlow(next_row);

                // Scroll the new row into view.
                const el = document.getElementById(next_row.session_id);
                if (el) {
                    el.scrollIntoView();
                    el.focus();
                }
            }
        };
    }
    gotoPrevRow = () => {
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        for(let i=0; i<this.node.table.props.data.length; i++) {
            let row = this.node.table.props.data[i];
            if (row.session_id === selected_flow) {
                // First row
                if(i===0){
                    return;
                }
                let prev_row = this.node.table.props.data[i-1];
                this.props.setSelectedFlow(prev_row);

                // Scroll the new row into view.
                const el = document.getElementById(prev_row.session_id);
                if (el) {
                    el.scrollIntoView();
                    el.focus();
                }
            }
        };
    }

    gotoTab = (tab) => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        this.props.history.push(
            "/collected/" + client_id + "/" + selected_flow + "/" + tab);
    }

    setFullScreen = () => {
        let client_id = this.props.selected_flow &&
            this.props.selected_flow.client_id;
        let selected_flow = this.props.selected_flow &&
            this.props.selected_flow.session_id;

        if (client_id && selected_flow) {
            this.props.history.push(
                "/fullscreen/collected/" + client_id + "/" +
                selected_flow + "/notebook");
        }
    }

    render() {
        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        let client_id = this.props.client && this.props.client.client_id;
        let columns = getFlowColumns(client_id);
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: row=>this.props.setSelectedFlow(row),
            selected: [selected_flow],
        };

        // When running on the server we have some special GUI.
        let isServer = client_id === "server";
        let KeyMap = {
            GOTO_RESULTS: {
                name: "Display server dashboard",
                sequence: "r",
            },
            GOTO_LOGS: "l",
            GOTO_OVERVIEW: "o",
            GOTO_UPLOADS: "u",
            NEXT: "n",
            PREVIOUS: "p",
            COLLECT: "c",
        };

        let keyHandlers={
            GOTO_RESULTS: (e)=>this.gotoTab("results"),
            GOTO_LOGS: (e)=>this.gotoTab("logs"),
            GOTO_UPLOADS: (e)=>this.gotoTab("uploads"),
            GOTO_OVERVIEW: (e)=>this.gotoTab("overview"),
            NEXT: this.gotoNextRow,
            PREVIOUS: this.gotoPrevRow,
            COLLECT: ()=>this.setState({showWizard: true}),
        };

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flow={this.props.selected_flow}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                      this.props.fetchFlows();
                  }}/>
              }
              { this.state.showSaveCollectionDialog &&
                <SaveCollectionDialog
                  flow={this.props.selected_flow}
                  onClose={e=>{
                      this.setState({showSaveCollectionDialog: false});
                  }}/>
              }
              { this.state.showWizard &&
                <NewCollectionWizard
                  client={this.props.client}
                  onCancel={(e) => this.setState({showWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showAddToHunt &&
                <AddFlowToHuntDialog
                  client={this.props.client}
                  flow={this.props.selected_flow}
                  onClose={e=>this.setState({showAddToHunt: false})}
                />
              }

              { this.state.showCopyWizard &&
                <NewCollectionWizard
                  client={this.props.client}
                  baseFlow={this.props.selected_flow}
                  onCancel={(e) => this.setState({showCopyWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showOfflineWizard &&
                <OfflineCollectorWizard
                  onCancel={(e) => this.setState({showOfflineWizard: false})}
                  onResolve={this.setCollectionRequest} />
              }

              { this.state.showDeleteNotebook &&
                <DeleteNotebookDialog
                  notebook_id={"N." + selected_flow + "-" + client_id}
                  onClose={(e) => this.setState({showDeleteNotebook: false})}/>
              }

              { this.state.showExportNotebook &&
                <ExportNotebook
                  notebook={{notebook_id: "N." + selected_flow + "-" + client_id}}
                  onClose={(e) => this.setState({showExportNotebook: false})}/>
              }

              <Navbar className="flow-toolbar">
                <ButtonGroup>
                  <Button title="New Collection"
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Add to hunt"
                          onClick={()=>this.setState({showAddToHunt: true})}
                          variant="default">
                    <FontAwesomeIcon icon="crosshairs"/>
                  </Button>

                  <Button title="Delete Artifact Collection"
                          onClick={()=>this.setState({showDeleteWizard: true}) }
                          variant="default">
                    <FontAwesomeIcon icon="eraser"/>
                  </Button>

                  { this.props.selected_flow.state === "RUNNING" &&
                    <Button title="Cancel Artifact Collection"
                            onClick={this.cancelButtonClicked}
                            variant="default">
                    <FontAwesomeIcon icon="stop"/>
                    </Button>
                  }
                  <Button title="Copy Collection"
                          onClick={() => this.setState({showCopyWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="copy"/>
                  </Button>
                  <Button title="Save Collection"
                          onClick={() => this.setState({
                              showSaveCollectionDialog: true
                          })}
                          variant="default">
                    <FontAwesomeIcon icon="save"/>
                  </Button>

                  { isServer &&
                    <Button title="Build offline collector"
                            onClick={() => this.setState({showOfflineWizard: true})}
                            variant="default">
                      <FontAwesomeIcon icon="paper-plane"/>
                    </Button>
                  }

                </ButtonGroup>
                { tab === "notebook" &&
                  <ButtonGroup className="float-right">
                    <Button title="Notebooks"
                            disabled={true}
                            variant="outline-dark">
                      <FontAwesomeIcon icon="book"/>
                    </Button>

                    <Button title="Full Screen"
                            onClick={this.setFullScreen}
                            variant="default">
                      <FontAwesomeIcon icon="expand"/>
                    </Button>

                    <Button title="Delete Notebook"
                            onClick={() => this.setState({showDeleteNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                    </Button>

                    <Button title="Export Notebook"
                            onClick={() => this.setState({showExportNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="download"/>
                    </Button>

                  </ButtonGroup>
                }
              </Navbar>

              <div className="fill-parent no-margins toolbar-margin selectable">
                <HotKeys keyMap={KeyMap} handlers={keyHandlers}>
                  <BootstrapTable
                    hover
                    condensed
                    ref={ n => this.node = n }
                    keyField="session_id"
                    bootstrap4
                    headerClasses="alert alert-secondary"
                    bodyClasses="fixed-table-body"
                    data={this.props.flows}
                    columns={columns}
                    selectRow={ selectRow }
                    filter={ filterFactory() }
                  />
                </HotKeys>
              </div>
            </>
        );
    }
};

export default withRouter(FlowsList);

export function getFlowColumns(client_id) {
    return formatColumns([
        {dataField: "state", text: "State", sort: true,
         formatter: (cell, row) => {
             if (cell === "FINISHED") {
                 return <FontAwesomeIcon icon="check"/>;
             }
             if (cell === "RUNNING") {
                 return <FontAwesomeIcon icon="hourglass"/>;
             }
             return <FontAwesomeIcon icon="exclamation"/>;
         }
        },
        {dataField: "session_id", text: "FlowId",
         formatter: (cell, row) => {
             return <NavLink
                      tabIndex="0"
                      id={cell}
                      to={"/collected/" + client_id + "/" + cell}>{cell}
        </NavLink>;
         }},
        {dataField: "request.artifacts", text: "Artifacts",
         sort: true, filtered: true,
         formatter: (cell, row) => {
             return _.map(cell, function(item, idx) {
                 return <div key={idx}>{item}</div>;
             });
         }},
        {dataField: "create_time", text: "Created", sort: true,
         type: "timestamp"},
        {dataField: "active_time", text: "Last Active", sort: true,
         type: "timestamp"},
        {dataField: "request.creator", text: "Creator",
         sort: true, filtered: true},
        {dataField: "total_uploaded_bytes", text: "Mb",
         align: 'right', sort: true, sortNumeric: true, type: "mb"},
        {dataField: "total_collected_rows", text: "Rows",
         sort: true, sortNumeric: true, align: 'right'}
    ]);
}
