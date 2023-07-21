import "./flows.css";

import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import _ from 'lodash';
import VeloPagedTable from '../core/paged-table.jsx';

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';

import api from '../core/api-service.jsx';
import { formatColumns } from "../core/table.jsx";
import NotebookUploads from '../notebooks/notebook-uploads.jsx';

import NewCollectionWizard from './new-collection.jsx';
import OfflineCollectorWizard from './offline-collector.jsx';
import DeleteNotebookDialog from '../notebooks/notebook-delete.jsx';
import ExportNotebook from '../notebooks/export-notebook.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { HotKeys } from "react-hotkeys";
import { withRouter } from "react-router-dom";
import { runArtifact } from "./utils.jsx";

import Modal from 'react-bootstrap/Modal';
import UserConfig from '../core/user.jsx';
import VeloForm from '../forms/form.jsx';
import AddFlowToHuntDialog from './flows-add-to-hunt.jsx';

import {CancelToken} from 'axios';

const POLL_TIME = 5000;

const SLIDE_STATES = [{
    level: "30%",
    icon: "arrow-down",
}, {
    level: "100%",
    icon: "arrow-up",
}, {
    level: "42px",
    icon: "arrows-up-down",
}];

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
        this.source = CancelToken.source();
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
            <Modal.Title>{T("Permanently delete collection")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("ArtifactDeletionDialog", this.props.flow.session_id,
                   artifacts, total_bytes, total_rows)}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.startDeleteFlow}>
                  {T("Yes do it!")}
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
        this.source = CancelToken.source();
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
                <Modal.Title>{T("Save this collection to your Favorites")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {T("ArtifactFavorites", artifacts)}
                <VeloForm
                  param={{name: "Name", description: T("New Favorite name")}}
                  value={this.state.name}
                  setValue={x=>this.setState({name:x})}
                />
                <VeloForm
                  param={{name: "Description",
                          description: T("Describe this favorite")}}
                  value={this.state.description}
                  setValue={x=>this.setState({description:x})}
                />

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.startSaveFlow}>
                  {T("Yes do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class FlowsList extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        client: PropTypes.object,
        flows: PropTypes.array,
        setSelectedFlow: PropTypes.func,
        selected_flow: PropTypes.object,
        collapseToggle: PropTypes.func,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        showWizard: false,
        showAddToHunt: false,
        showCopyWizard: false,
        showOfflineWizard: false,
        showDeleteWizard: false,
        showDeleteNotebook: false,
        initialized_from_parent: false,
        selectedFlowId: undefined,
        version: {version: 0},
        slider: 0,
        transform: undefined,
    }

    incrementVersion = () => {
        this.setState({version: {version: this.state.version.version+1}});
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.incrementVersion, POLL_TIME);

        let slider = SLIDE_STATES[this.state.slider];
        this.props.collapseToggle(slider.level);

        let action = this.props.match && this.props.match.params &&
            this.props.match.params.flow_id;

        let client_id = this.props.match && this.props.match.params &&
            this.props.match.params.client_id;

        let name = this.props.match && this.props.match.params &&
            this.props.match.params.tab;

        if (_.isUndefined(client_id)) {
            client_id = "server";
        }

        if (action === "new") {
            let specs = {};
            specs[name] = {};

            let initial_flow = {
                request: {
                    client_id: client_id,
                    artifacts: [name],
                },
            };
            this.setState({
                showNewFromRouterWizard: true,
                client_id: client_id,
                initial_flow: initial_flow,
            });
            this.props.history.push("/collected/" + client_id);
        }
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        if (this.interval) {
            clearInterval(this.interval);
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
        api.post("v1/CollectArtifact",
                 request, this.source.token).then((response) => {
                     // Only disable wizards if the request was successful.
                     this.setState({showWizard: false,
                                    showOfflineWizard: false,
                                    showNewFromRouterWizard: false,
                                    showCopyWizard: false});
                     this.incrementVersion();
                 });
    }

    cancelButtonClicked = () => {
        let client_id = this.props.selected_flow && this.props.selected_flow.client_id;
        let flow_id = this.props.selected_flow && this.props.selected_flow.session_id;

        if (client_id && flow_id) {
            api.post("v1/CancelFlow", {
                client_id: client_id, flow_id: flow_id
            }, this.source.token).then((response) => {
                this.incrementVersion();
            });
        }
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

    expandSlider = ()=>{
        let next_slide = (this.state.slider + 1) % SLIDE_STATES.length;
        this.setState({ slider: next_slide});
        this.props.collapseToggle(SLIDE_STATES[next_slide].level);
    };

    render() {
        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        let client_id = this.props.client && this.props.client.client_id;
        let selected_flow = this.props.selected_flow && this.props.selected_flow.session_id;
        let username = this.context &&
            this.context.traits && this.context.traits.username;

        const selectRow = {
            mode: "radio",
            clickToSelect: true,
            hideSelectColumn: true,
            classes: "row-selected",
            onSelect: row=>{
                this.props.setSelectedFlow(row._Flow);
                this.setState({selectedFlowId: row._id});
            },
            selected: [this.state.selectedFlowId],
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
            COLLECT: "c",
        };

        let keyHandlers={
            GOTO_RESULTS: (e)=>this.gotoTab("results"),
            GOTO_LOGS: (e)=>this.gotoTab("logs"),
            GOTO_UPLOADS: (e)=>this.gotoTab("uploads"),
            GOTO_OVERVIEW: (e)=>this.gotoTab("overview"),
            COLLECT: ()=>this.setState({showWizard: true}),
        };

        let renderers = {
            State: stateRenderer,
            Artifacts: (cell, row) => {
                return _.map(cell, function(item, idx) {
                    return <div key={idx}>{item}</div>;
                });
            },
        };

        return (
            <>
              { this.state.showDeleteWizard &&
                <DeleteFlowDialog
                  client={this.props.client}
                  flow={this.props.selected_flow}
                  onClose={e=>{
                      this.setState({showDeleteWizard: false});
                      this.incrementVersion();
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

              { this.state.showNewFromRouterWizard &&
                <NewCollectionWizard
                  client={{client_id: this.state.client_id}}
                  baseFlow={this.state.initial_flow}
                  onCancel={(e) => this.setState({showNewFromRouterWizard: false})}
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

              { this.state.showNotebookUploadsDialog &&
                <NotebookUploads
                  notebook={{notebook_id: "N." + selected_flow + "-" + client_id}}
                  closeDialog={(e) => this.setState({showNotebookUploadsDialog: false})}/>
              }

              { this.state.showExportNotebook &&
                <ExportNotebook
                  notebook={{notebook_id: "N." + selected_flow + "-" + client_id}}
                  onClose={(e) => this.setState({showExportNotebook: false})}/>
              }

              <Navbar className="flow-toolbar">
                <ButtonGroup>
                  <Button data-tooltip={T("New Collection")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => this.setState({showWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                    <span className="sr-only">{T("New Collection")}</span>
                  </Button>

                  { client_id !== "server" &&
                    <Button data-tooltip={T("Add to hunt")}
                            data-position="right"
                            className="btn-tooltip"
                            onClick={()=>this.setState({showAddToHunt: true})}
                            variant="default">
                      <FontAwesomeIcon icon="crosshairs"/>
                      <span className="sr-only">{T("Add to hunt")}</span>
                    </Button>
                  }
                  <Button data-tooltip={T("Delete Artifact Collection")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={()=>this.setState({showDeleteWizard: true}) }
                          variant="default">
                    <FontAwesomeIcon icon="trash-alt"/>
                    <span className="sr-only">{T("Delete Artifact Collection")}</span>
                  </Button>

                  <Button data-tooltip={T("Cancel Artifact Collection")}
                          data-position="right"
                          disabled={this.props.selected_flow.state === "FINISHED" ||
                                    this.props.selected_flow.state === "ERROR"}
                          className="btn-tooltip"
                          onClick={this.cancelButtonClicked}
                          variant="default">
                    <FontAwesomeIcon icon="stop"/>
                    <span className="sr-only">{T("Cancel Artifact Collection")}</span>
                  </Button>

                  <Button data-tooltip={T("Copy Collection")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => this.setState({showCopyWizard: true})}
                          variant="default">
                    <FontAwesomeIcon icon="copy"/>
                    <span className="sr-only">{T("Copy Collection")}</span>
                  </Button>
                  <Button data-tooltip={T("Save Collection")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => this.setState({
                              showSaveCollectionDialog: true
                          })}
                          variant="default">
                    <FontAwesomeIcon icon="save"/>
                    <span className="sr-only">{T("Save Collection")}</span>
                  </Button>

                  <Button data-tooltip={T("Stats Toggle")}
                          data-position="left"
                          className="btn-tooltip"
                          variant="default"
                          onClick={this.expandSlider}>
                    <FontAwesomeIcon icon={SLIDE_STATES[this.state.slider].icon}/>
                  </Button>

                  { _.isEmpty(this.state.transform) ?
                    <Button data-tooltip={T("Show only my collections")}
                            data-position="right"
                            className="btn-tooltip"
                            onClick={()=>{
                                this.setState({transform: {
                                    filter_regex: username,
                                    filter_column: "Creator"}});
                                this.incrementVersion();
                            }}
                            variant="default">
                      <FontAwesomeIcon icon="user" />
                      <span className="sr-only">{T("Show only my hunts")}</span>
                    </Button>
                    :
                    <Button data-tooltip={T("Show all collections")}
                            data-position="right"
                            className="btn-tooltip"
                            onClick={()=>{
                                this.setState({transform: {}});
                                this.incrementVersion();
                            }}
                            variant="default">
                      <FontAwesomeIcon icon="user-large-slash" />
                      <span className="sr-only">{T("Show all hunts")}</span>
                    </Button>
                  }

                  { isServer &&
                    <Button data-tooltip={T("Build offline collector")}
                            data-position="right"
                            className="btn-tooltip"
                            onClick={() => this.setState({showOfflineWizard: true})}
                            variant="default">
                      <FontAwesomeIcon icon="paper-plane"/>
                      <span className="sr-only">{T("Build offline collector")}</span>
                    </Button>
                  }

                </ButtonGroup>
                { tab === "notebook" &&
                  <ButtonGroup className="float-right">
                    <Button data-tooltip={T("Notebooks")}
                            data-position="left"
                            className="btn-tooltip"
                            disabled={true}
                            variant="outline-dark">
                      <FontAwesomeIcon icon="book"/>
                      <span className="sr-only">{T("Notebooks")}</span>
                    </Button>

                    <Button data-tooltip={T("Full Screen")}
                            data-position="left"
                            className="btn-tooltip"
                            onClick={this.setFullScreen}
                            variant="default">
                      <FontAwesomeIcon icon="expand"/>
                      <span className="sr-only">{T("Full Screen")}</span>
                    </Button>

                    <Button data-tooltip={T("Delete Notebook")}
                            data-position="left"
                            className="btn-tooltip"
                            onClick={() => this.setState({showDeleteNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                      <span className="sr-only">{T("Delete Notebook")}</span>
                    </Button>

                    <Button data-tooltip={T("Notebook Uploads")}
                            data-position="left"
                            className="btn-tooltip"
                            onClick={() => this.setState({showNotebookUploadsDialog: true})}
                            variant="default">
                      <FontAwesomeIcon icon="fa-file-download"/>
                      <span className="sr-only">{T("Notebook Uploads")}</span>
                    </Button>

                    <Button data-tooltip={T("Export Notebook")}
                            data-position="left"
                            className="btn-tooltip"
                            onClick={() => this.setState({showExportNotebook: true})}
                            variant="default">
                      <FontAwesomeIcon icon="download"/>
                      <span className="sr-only">{T("Export Notebook")}</span>
                    </Button>

                  </ButtonGroup>
                }
              </Navbar>

              <div className="fill-parent no-margins toolbar-margin selectable">
                <HotKeys keyMap={KeyMap} handlers={keyHandlers}>
                  <VeloPagedTable
                    url="v1/GetClientFlows"
                    params={{client_id: client_id}}
                    translate_column_headers={true}
                    prevent_transformations={{
                        Mb: true, Rows: true,
                        State: true, "Last Active": true}}
                    selectRow={selectRow}
                    renderers={renderers}
                    version={this.state.version}
                    no_spinner={true}
                    row_classes={rowClassRenderer}
                    transform={this.state.transform}
                    setTransform={x=>{
                        this.setState({transform: x});
                    }}
                    no_toolbar={true}
                  />
                </HotKeys>
              </div>
            </>
        );
    }
};

export default withRouter(FlowsList);


const stateRenderer = (cell, row) => {
    let result = <></>;

    if (cell === "FINISHED") {
        result = <FontAwesomeIcon icon="check"/>;

    } else if (cell === "RUNNING") {
        result = <FontAwesomeIcon icon="hourglass"/>;

    } else {
        result = <FontAwesomeIcon icon="exclamation"/>;
    }

    return <div className="flow-status-icon">{result}</div>;
};

const rowClassRenderer = (row, rowIndex) => {
    if (row._Urgent === "true") {
        return 'flow-urgent';
    }
    return '';
};

export function getFlowColumns(client_id) {
    return formatColumns([
        {dataField: "state", text: T("State"), sort: true,
         formatter: stateRenderer,
        },
        {dataField: "session_id", text: T("FlowId"), type: "flow"},
        {dataField: "request.artifacts", text: T("Artifacts"),
         sort: true, filtered: true,
         formatter: (cell, row) => {
             return _.map(cell, function(item, idx) {
                 return <div key={idx}>{item}</div>;
             });
         }},
        {dataField: "create_time", text: T("Created"), sort: true,
         type: "timestamp"},
        {dataField: "active_time", text: T("Last Active"), sort: true,
         type: "timestamp"},
        {dataField: "request.creator", text: T("Creator"),
         sort: true, filtered: true},
        {dataField: "total_uploaded_bytes", text: T("Mb"),
         align: 'right', sort: true, sortNumeric: true, type: "mb"},
        {dataField: "total_collected_rows", text: T("Rows"),
         sort: true, sortNumeric: true, align: 'right'}
    ]);
}
