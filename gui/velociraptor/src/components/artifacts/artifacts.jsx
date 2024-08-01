import './artifacts.css';
import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import VeloReportViewer from "../artifacts/reporting.jsx";
import Select from 'react-select';
import Spinner from '../utils/spinner.jsx';

import _ from 'lodash';
import {CancelToken} from 'axios';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import InputGroup from 'react-bootstrap/InputGroup';
import Form from 'react-bootstrap/Form';
import Table  from 'react-bootstrap/Table';
import NewArtifactDialog from './new-artifact.jsx';
import Container from  'react-bootstrap/Container';
import ArtifactsUpload from './artifacts-upload.jsx';
import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";
import ToolTip from '../widgets/tooltip.jsx';

import SplitPane from 'react-split-pane';

const presetFilters = ()=>[
    {value: "type:CLIENT", label: T("Client Artifacts")},
    {value: "type:SERVER", label: T("Server Artifacts")},
    {value: "type:NOTEBOOK", label: T("Notebook templates")},
    {value: "", label: T("All Artifacts")},
    {value: "precondition:WINDOWS", label: T("Windows Only")},
    {value: "precondition:LINUX", label: T("Linux Only")},
    {value: "precondition:DARWIN", label: T("macOS Only")},
    {value: "type:CLIENT_EVENT", label: T("Client Monitoring")},
    {value: "type:SERVER_EVENT", label: T("Server Monitoring")},
    {value: "tool:.+", label: T("Using Tools")},
    {value: "^exchange.+", label: T("Exchange")},
    {value: "builtin:yes", label: T("BuiltIn Only")},
    {value: "builtin:no", label: T("Custom Only")},
    {value: "metadata:basic", label: T("Basic Only")},
];


class DeleteOKDialog extends React.Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        onAccept: PropTypes.func.isRequired,
        name: PropTypes.string,
    }

    render() {
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Delete an artifact")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>{T("You are about to delete", this.props.name)}</Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.props.onAccept}>
                  {T("Yes do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class ArtifactInspector extends React.Component {
    static propTypes = {
        client: PropTypes.object,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        selectedDescriptor: undefined,
        fullSelectedDescriptor: {},

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        timeout: 0,

        loading: false,

        showNewArtifactDialog: false,
        showEditedArtifactDialog: false,
        showDeleteArtifactDialog: false,
        showArtifactsUploadDialog: false,

        // The current filter string in the search box.
        current_filter: "",

        // The currently selected preset filter.
        preset_filter: "type:CLIENT",

        version: 0,

        filter_name: T("Client Artifacts"),
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.searchInput.focus();
        let artifact_name = this.props.match && this.props.match.params &&
            this.props.match.params.artifact;

        if (!artifact_name) {
            this.fetchRows("...", this.state.preset_filter);
            return;
        }
        // If we get here the url contains the artifact name, we
        // therefore have to allow all types of artifacts because we
        // dont know which type the artifact is
        this.setState({selectedDescriptor: {name: artifact_name},
                       preset_filter: "",
                       current_filter: artifact_name});
        this.updateSearch(artifact_name);
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    updateSearch = (value) => {
        this.setState({current_filter: value});
        this.fetchRows(value, this.state.preset_filter);
    }

    fetchRows = (search_term, preset_filter) => {
        this.setState({loading: true});

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        api.post("v1/GetArtifacts",
                {
                    search_term: _.trim(preset_filter) + " " +
                        _.trim(search_term|| "..."),
                    // This might be too many to fetch at once but we
                    // are still fast enough for now.
                    fields: {
                        name: true,
                    },
                    number_of_results: 1000,
                },
                this.source.token).then((response) => {
                    if (response.cancel) return;

                    let matchingDescriptors = [];
                    let items = response.data.items || [];

                    for(let i=0; i<items.length; i++) {
                        var desc = items[i];
                        matchingDescriptors.push(desc);
                    };

                    this.setState({
                        matchingDescriptors: matchingDescriptors,
                        loading: false,
                    });
                });
    }

    onSelect = (row, e) => {
        this.setState({
            selectedDescriptor: row,
            fullSelectedDescriptor: {},
        });
        this.props.history.push("/artifacts/" + row.name);
        e.preventDefault();
        e.stopPropagation();

        return this.getArtifactDescription(row.name);
   }

    getArtifactDescription = (name) => {
        // Fetch the full description
        api.post("v1/GetArtifacts", {
            names: [name],
        }, this.source.token).then(response=>{
            let items = response.data.items;
            if (items.length > 0) {
                this.setState({
                    fullSelectedDescriptor: items[0],
                    version: this.state.version+1,
                });
            };
        });
        return false;
    }

    huntArtifactEnabled = ()=>{
        if (!this.state.selectedDescriptor) {
            return false;
        }

        if (this.state.fullSelectedDescriptor.type !== "client") {
            return false;
        }
        return true;
    }

    huntArtifact = () => {
        this.props.history.push("/hunts/new/" + this.state.selectedDescriptor.name);
    }

    collectArtifactEnabled = ()=>{
        if (!this.state.selectedDescriptor) {
            return false;
        }

        let type = this.state.fullSelectedDescriptor.type;
        if (type !== "client" && type !== "server") {
            return false;
        }

        // If a client is not selected we can not pivot to it.
        if (type === "client") {
            if (!this.props.client || !this.props.client.client_id) {
                return false;
            }
        }

        return true;
    }
    collectArtifact = ()=>{
        let client_id = this.props.client && this.props.client.client_id;
        let name = this.state.selectedDescriptor.name;
        let type = this.state.fullSelectedDescriptor.type;

        if (type === "server") {
            client_id = "server";
        }

        this.props.history.push("/collected/" + client_id + "/new/" + name);
    }

    deleteArtifact = (selected) => {
        api.post('v1/SetArtifactFile', {
            artifact: "name: "+selected,
            op: "DELETE",
        }, this.source.token).then(resp => {
            if (resp.cancel) return;

            this.fetchRows(this.state.current_filter, this.state.preset_filter);
            this.setState({showDeleteArtifactDialog: false});
        });
    }

    renderFilter = ()=>{
        let option_value = _.filter(presetFilters(), x=>{
            return x.label === this.state.filter_name;
        });
        return <Select
                 className="artifact-filter"
                 classNamePrefix="velo"
                 options={presetFilters()}
                 value={option_value}
                 onChange={x=>{
                     this.setState({
                         preset_filter: x.value,
                         filter_name: x.label,
                     });
                     this.fetchRows(this.state.current_filter, x.value);
                 }}
                 placeholder={T("Filter artifact")}
               />;
    }

    getItemIcon = item=>{
        if (!item.built_in && !item.is_inherited) {
            return <FontAwesomeIcon icon="user-edit" /> ;
        }

        if (!item.built_in && item.is_inherited) {
            return <FontAwesomeIcon icon="house" /> ;
        }

        return <span className="invisible">
                 <FontAwesomeIcon icon="user-edit" />
               </span>;
    };

    render() {
        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let deletable = this.state.selectedDescriptor &&
            !this.state.selectedDescriptor.built_in &&
            !this.state.selectedDescriptor.is_inherited;

        return (
            <div className="full-width-height"><Spinner loading={this.state.loading}/>
              { this.state.showNewArtifactDialog &&
                <NewArtifactDialog
                  onClose={() => {
                      // Re-apply the search in case the user updated
                      // an artifact that should show up.
                      this.fetchRows(this.state.current_filter,
                                     this.state.preset_filter);
                      this.setState({showNewArtifactDialog: false});
                  }}
                />
              }

              { this.state.showEditedArtifactDialog &&
                <NewArtifactDialog
                  name={selected}
                  onClose={() => {
                      // Re-apply the search in case the user updated
                      // an artifact that should show up.
                      this.fetchRows(this.state.current_filter,
                                     this.state.preset_filter);
                      this.getArtifactDescription(selected);
                      this.setState({showEditedArtifactDialog: false});
                  }}
                />
              }

              { this.state.showDeleteArtifactDialog &&
                <DeleteOKDialog
                  name={selected}
                  onClose={()=>this.setState({showDeleteArtifactDialog: false})}
                  onAccept={()=>this.deleteArtifact(selected)}
                />
              }
              { this.state.showArtifactsUploadDialog &&
                <ArtifactsUpload
                  onClose={() => {
                      // Re-apply the search in case the user updated
                      // an artifact that should show up.
                      this.fetchRows(this.state.current_filter,
                                     this.state.preset_filter);
                      this.setState({showArtifactsUploadDialog: false});
                  }}
                />
              }
              <Navbar className="artifact-toolbar justify-content-between">
                <ButtonGroup>
                  <ToolTip tooltip={T("Add an Artifact")}>
                    <Button onClick={() => this.setState({showNewArtifactDialog: true})}
                            variant="default">
                      <FontAwesomeIcon icon="plus"/>
                      <span className="sr-only">{T("Add an Artifact")}</span>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Edit an Artifact")} >
                    <Button onClick={() => {
                                this.setState({showEditedArtifactDialog: true});
                            }}
                            disabled={!selected}
                            variant="default">
                      <FontAwesomeIcon icon="pencil-alt"/>
                      <span className="sr-only">{T("Edit an Artifact")}</span>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Delete Artifact")} >
                    <Button onClick={() => this.setState({showDeleteArtifactDialog: true})}
                            disabled={!deletable}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                      <span className="sr-only">{T("Delete Artifact")}</span>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Hunt Artifact")} >
                    <Button onClick={this.huntArtifact}
                            disabled={!this.huntArtifactEnabled()}
                            variant="default">
                      <FontAwesomeIcon icon="crosshairs"/>
                      <span className="sr-only">{T("Hunt Artifact")}</span>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Collect Artifact")} >
                    <Button onClick={this.collectArtifact}
                            disabled={!this.collectArtifactEnabled()}
                            variant="default">
                      <FontAwesomeIcon icon="cloud-download-alt"/>
                      <span className="sr-only">{T("Collect Artifact")}</span>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Upload Artifact Pack")} >
                    <Button onClick={()=>this.setState({showArtifactsUploadDialog: true})}
                            variant="default">
                      <FontAwesomeIcon icon="upload"/>
                      <span className="sr-only">{T("Upload Artifact Pack")}</span>
                    </Button>
                  </ToolTip>
                </ButtonGroup>
                <Form className="artifact-search">
                  <InputGroup>
                    { this.renderFilter() }
                    <Form.Control className="artifact-search-input"
                                 ref={(input) => { this.searchInput = input; }}
                                 value={this.state.current_filter}
                                 onChange={(e) => this.updateSearch(
                                     e.currentTarget.value)}
                                 placeholder={T("Search for artifact")}
                                 spellCheck="false"
                    />
                    <ToolTip tooltip={T("Clear")}  >
                      <Button onClick={()=>this.updateSearch("")}
                              variant="light">
                        <FontAwesomeIcon icon="broom"/>
                        <span className="sr-only">{T("Clear")}</span>
                      </Button>
                    </ToolTip>
                  </InputGroup>
                </Form>
              </Navbar>
              <div className="artifact-search-panel">
                <SplitPane
                  split="vertical"
                  defaultSize="70%">
                  <div className="artifact-search-report">
                    { this.state.selectedDescriptor ?
                      <VeloReportViewer
                        artifact={this.state.selectedDescriptor.name}
                        params={{version: this.state.version}}
                        type="ARTIFACT_DESCRIPTION"
                        client={{client_id: this.state.selectedDescriptor.name}}
                      /> :
                      <h2 className="no-content">{T("Search for an artifact to view it")}</h2>
                    }
                  </div>
                  <Container className="artifact-search-table selectable">
                    <Table  hover size="sm">
                      <tbody>
                        { _.map(this.state.matchingDescriptors, (item, idx) => {
                            return <tr key={idx} className={
                                this.state.selectedDescriptor &&
                                    item.name === this.state.selectedDescriptor.name ?
                                    "row-selected" : undefined
                            }>
                                     <td>
                                       <button type="button"
                                               href="#"
                                               className="link-button"
                                               onClick={e=>this.onSelect(item, e)}>
                                         {item.name}
                                         <span className="built-in-icon">
                                           { this.getItemIcon(item) }
                                         </span>
                                       </button>

                                     </td>
                                   </tr>;
                        })}
                      </tbody>
                    </Table>
                  </Container>

                </SplitPane>
              </div>
            </div>
        );
    }
};

export default withRouter(ArtifactInspector);
