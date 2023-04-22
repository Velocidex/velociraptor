import './artifacts.css';
import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import classNames from "classnames";
import VeloReportViewer from "../artifacts/reporting.jsx";
import Select from 'react-select';

import _ from 'lodash';
import axios from 'axios';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import InputGroup from 'react-bootstrap/InputGroup';
import Form from 'react-bootstrap/Form';
import FormControl from 'react-bootstrap/FormControl';
import Table  from 'react-bootstrap/Table';
import NewArtifactDialog from './new-artifact.jsx';
import Container from  'react-bootstrap/Container';
import ArtifactsUpload from './artifacts-upload.jsx';
import T from '../i8n/i8n.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";

import SplitPane from 'react-split-pane';

const presetFilters = [
    {value: "type:CLIENT", label: T("Client Artifacts")},
    {value: "", label: T("All Artifacts")},
    {value: "precondition:WINDOWS", label: T("Windows Only")},
    {value: "precondition:LINUX", label: T("Linux Only")},
    {value: "precondition:DARWIN", label: T("OSX Only")},
    {value: "type:CLIENT_EVENT", label: T("Client Monitoring")},
    {value: "type:SERVER_EVENT", label: T("Servr Monitoring")},
    {value: "type:SERVER", label: T("Server Artifacts")},
    {value: "tool:.+", label: T("Using Tools")},
    {value: "^exchange.+", label: T("Exchange")},
    {value: "^custom.+", label: T("Custom")},
    {value: "builtin:yes", label: T("BuiltIn Only")},
    {value: "builtin:no", label: T("Custom Only")},
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
        current_filter: "",

        version: 0,

        filter_name: T("Client Artifacts"),
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.searchInput.focus();
        let artifact_name = this.props.match && this.props.match.params &&
            this.props.match.params.artifact;

        if (!artifact_name) {
            this.fetchRows("...");
            return;
        }
        this.setState({selectedDescriptor: {name: artifact_name},
                       current_filter: artifact_name});
        this.updateSearch(artifact_name);
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    updateSearch = (value) => {
        this.setState({current_filter: value});
        this.fetchRows(value);
    }

    fetchRows = (search_term) => {
        this.setState({loading: true});

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.post("v1/GetArtifacts",
                {
                    search_term: search_term || "...",
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
            this.fetchRows(this.state.current_filter);
            this.setState({showDeleteArtifactDialog: false});
        });
    }

    renderFilter = ()=>{
        let option_value = _.filter(presetFilters, x=>{
            return x.label === this.state.filter_name;
        });
        return <Select
                 className="org-selector artifact-filter"
                 classNamePrefix="velo"
                 options={presetFilters}
                 value={option_value}
                 onChange={x=>{
                     this.setState({
                         current_filter: x.value,
                         filter_name: x.label,
                     });
                     this.fetchRows(x.value);
                 }}
                 placeholder={T("Filter artifact")}
               />;
    }

    render() {
        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let deletable = this.state.selectedDescriptor &&
            !this.state.selectedDescriptor.built_in;

        return (
            <div className="full-width-height">
              { this.state.showNewArtifactDialog &&
                <NewArtifactDialog
                  onClose={() => {
                      // Re-apply the search in case the user updated
                      // an artifact that should show up.
                      this.fetchRows(this.state.current_filter);
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
                      this.fetchRows(this.state.current_filter);
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
                      this.fetchRows(this.state.current_filter);
                      this.setState({showArtifactsUploadDialog: false});
                  }}
                />
              }
              <Navbar className="artifact-toolbar justify-content-between">
                <ButtonGroup>
                  <Button data-tooltip={T("Add an Artifact")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => this.setState({showNewArtifactDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
            <span className="sr-only">{T("Add an Artifact")}</span>
                  </Button>

                  <Button data-tooltip={T("Edit an Artifact")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => {
                              this.setState({showEditedArtifactDialog: true});
                          }}
                          disabled={!selected}
                          variant="default">
                    <FontAwesomeIcon icon="pencil-alt"/>
            <span className="sr-only">{T("Edit an Artifact")}</span>
                  </Button>

                  <Button data-tooltip={T("Delete Artifact")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={() => this.setState({showDeleteArtifactDialog: true})}
                          disabled={!deletable}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
            <span className="sr-only">{T("Delete Artifact")}</span>
                  </Button>

                  <Button data-tooltip={T("Hunt Artifact")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={this.huntArtifact}
                          disabled={!this.huntArtifactEnabled()}
                          variant="default">
                    <FontAwesomeIcon icon="crosshairs"/>
            <span className="sr-only">{T("Hunt Artifact")}</span>
                  </Button>

                  <Button data-tooltip={T("Collect Artifact")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={this.collectArtifact}
                          disabled={!this.collectArtifactEnabled()}
                          variant="default">
                    <FontAwesomeIcon icon="cloud-download-alt"/>
            <span className="sr-only">{T("Collect Artifact")}</span>
                  </Button>

                  <Button data-tooltip={T("Upload Artifact Pack")}
                          data-position="right"
                          className="btn-tooltip"
                          onClick={()=>this.setState({showArtifactsUploadDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="upload"/>
            <span className="sr-only">{T("Upload Artifact Pack")}</span>
                  </Button>
                </ButtonGroup>
                <Form inline className="artifact-search">
                  <InputGroup >
                    <InputGroup.Prepend>
                        { this.renderFilter() }
                      <FormControl className="artifact-search-input"
                                   ref={(input) => { this.searchInput = input; }}
                                   value={this.state.current_filter}
                                   onChange={(e) => this.updateSearch(e.currentTarget.value)}
                                   placeholder={T("Search for artifact")}
                                   spellCheck="false"
                      />
                    </InputGroup.Prepend>
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
                    <Table  bordered hover size="sm">
                      <tbody>
                        { _.map(this.state.matchingDescriptors, (item, idx) => {
                            return <tr key={idx} className={
                                this.state.selectedDescriptor &&
                                    item.name === this.state.selectedDescriptor.name ?
                                    "row-selected" : undefined
                            }>
                                     <td onClick={(e) => this.onSelect(item, e)}>
                                       {/* eslint jsx-a11y/anchor-is-valid: "off" */}
                                       <a href="#"
                                          onClick={(e) => this.onSelect(item, e)}>
                                         {item.name}
                                       </a>
                                       <span className="user-edit">
                                         <FontAwesomeIcon
                                           className={classNames({"invisible": item.built_in})}
                                           icon="user-edit" />
                                       </span>
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
