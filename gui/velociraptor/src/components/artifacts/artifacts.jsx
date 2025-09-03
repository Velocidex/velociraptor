import './artifacts.css';
import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import VeloReportViewer from "../artifacts/reporting.jsx";
import Select from 'react-select';
import Spinner from '../utils/spinner.jsx';
import Alert from 'react-bootstrap/Alert';

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
    {value: "builtin:yes", label: T("BuiltIn Only")},
    {value: "builtin:no", label: T("Custom Only")},
    {value: "metadata:basic", label: T("Basic Only")},
    {value: "empty:true", label: T("Include Empty sources")},
];


class DeleteOKDialog extends React.Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
        onAccept: PropTypes.func.isRequired,
        names: PropTypes.array,
    }

    render() {
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Delete artifacts")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Alert variant="danger">
                  {T("You are about to delete the following artifacts")}
                </Alert>
                <table className="deleting-artifacts">
                  <tbody>
                    {_.map(this.props.names, (name, idx)=>{
                        return <tr key={idx}><td>{name}</td></tr>;
                    })}
                  </tbody>
                </table>

              </Modal.Body>
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
        multiSelectedDescriptors: [],
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

        // All the knowns tags for artifacts.
        tags: [],
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
        this.getArtifactDescription(artifact_name);

        if (this.props.match && this.props.match.params &&
            this.props.match.params.action === "edit") {
            this.setState({showEditedArtifactDialog: true});
        }
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
                        tags: true,
                    },
                    number_of_results: 1000,
                },
                this.source.token).then((response) => {
                    if (response.cancel) return;

                    let matchingDescriptors = [];
                    let items = response.data.items || [];

                    for(let i=0; i<items.length; i++) {
                        var desc = items[i];
                        desc.idx = i;
                        matchingDescriptors.push(desc);
                    };

                    this.setState({
                        tags: response.data.tags,
                        matchingDescriptors: matchingDescriptors,
                        loading: false,
                    });
                });
    }

    onSelect = (row, e) => {
        if (e.shiftKey) {
            document.getSelection().removeAllRanges();
            let first = 0;
            if (!_.isEmpty(this.state.selectedDescriptor)) {
                first = this.state.selectedDescriptor.idx;
            };

            if (!_.isEmpty(this.state.multiSelectedDescriptors) &&
                this.state.multiSelectedDescriptors[0].idx < first ){
                first = this.state.multiSelectedDescriptors[0].idx;
            }

            let current = row.idx;
            if(current < first) {
                let tmp = first;
                first = current;
                current = tmp;
            };
            let multi_selection = [];
            for (let i=first;i<=current;i++) {
                multi_selection.push(this.state.matchingDescriptors[i]);
            }
            this.setState({multiSelectedDescriptors: multi_selection});

        } else {
            // Clear the selection.
            this.setState({multiSelectedDescriptors: []});
        }

        this.setState({
            selectedDescriptor: row,
            fullSelectedDescriptor: {},
            version: this.state.version+1,
        });
        this.props.history.push("/artifacts/" + row.name);
        e.preventDefault();
        e.stopPropagation();

        return this.getArtifactDescription(row.name);
   }

    getClassName = item=>{
        if (this.state.selectedDescriptor &&
            item.name === this.state.selectedDescriptor.name) {
            return "row-selected";
        }
        if (_.isEmpty(this.state.multiSelectedDescriptors)) {
            return "";
        }
        let first = this.state.multiSelectedDescriptors[0];
        let last = this.state.multiSelectedDescriptors[
            this.state.multiSelectedDescriptors.length-1];

        if (item.idx >= first.idx && item.idx <= last.idx) {
            return "row-selected row-multi-selected";
        }
        return "";
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

    deleteArtifacts = (selected) => {
        let counter = 0;
        _.each(selected, name=>{
            counter++;
            api.post('v1/SetArtifactFile', {
                artifact: "name: "+name,
                op: "DELETE",
            }, this.source.token).then(resp => {
                if (resp.cancel) return;
                counter--;
                if (counter==0) {
                    this.fetchRows(
                        this.state.current_filter, this.state.preset_filter);
                    this.setState({showDeleteArtifactDialog: false});
                };
            });
        });
    }

    renderFilter = ()=>{
        let filters = presetFilters();
        _.each(this.state.tags, x=>{
           filters.push({value: "tag:" + x, label: T("Tag") + " " + x});
        });

        let option_value = _.filter(filters, x=>{
            return x.label === this.state.filter_name;
        });
        return <Select
                 className="artifact-filter"
                 classNamePrefix="velo"
                 options={filters}
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

    isDeletable = descriptor=>{
        return descriptor &&
            !descriptor.built_in &&
            !descriptor.is_alias &&
            !descriptor.is_inherited;
    }

    render() {
        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let deletable = [];
        _.each(this.state.multiSelectedDescriptors, item=>{
            if(this.isDeletable(item)) {
                deletable.push(item.name);
            };
        });
        if(_.isEmpty(deletable) &&
           this.isDeletable(this.state.selectedDescriptor)) {
            deletable.push(this.state.selectedDescriptor.name);
        };

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
                      this.setState({
                          showEditedArtifactDialog: false,
                          version: this.state.version+1,
                      });
                  }}
                />
              }

              { this.state.showDeleteArtifactDialog &&
                <DeleteOKDialog
                  names={deletable}
                  onClose={()=>this.setState({showDeleteArtifactDialog: false})}
                  onAccept={()=>this.deleteArtifacts(deletable)}
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
                            disabled={_.isEmpty(deletable)}
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
                            return <tr key={idx}
                                       className={this.getClassName(item)}>
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
