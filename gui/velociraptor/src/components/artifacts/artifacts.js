import './artifacts.css';
import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.js';

import VeloReportViewer from "../artifacts/reporting.js";

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
import NewArtifactDialog from './new-artifact.js';
import Container from  'react-bootstrap/Container';
import ArtifactsUpload from './artifacts-upload.js';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { withRouter }  from "react-router-dom";

import SplitPane from 'react-split-pane';

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
                <Modal.Title>Delete an artifact</Modal.Title>
              </Modal.Header>
              <Modal.Body>You are about to delete {this.props.name}</Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  Close
                </Button>
                <Button variant="primary" onClick={this.props.onAccept}>
                  Yes do it!
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

class ArtifactInspector extends React.Component {
    static propTypes = {

    };

    state = {
        selectedDescriptor: undefined,

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        timeout: 0,

        loading: false,

        showNewArtifactDialog: false,
        showEditedArtifactDialog: false,
        showDeleteArtifactDialog: false,
        showArtifactsUploadDialog: false,
        current_filter: "",
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
                    // This might be too many to fetch at once but we are still fast enough for now.
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
        this.setState({selectedDescriptor: row});
        this.props.history.push("/artifacts/" + row.name);
        e.preventDefault();
        e.stopPropagation();
        return false;
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

    render() {
        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let deletable = selected && selected.match(/^Custom/);

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
                  <Button title="Add an Artifact"
                          onClick={() => this.setState({showNewArtifactDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="plus"/>
                  </Button>

                  <Button title="Edit an Artifact"
                          onClick={() => {
                              this.setState({showEditedArtifactDialog: true});
                          }}
                          disabled={!selected}
                          variant="default">
                    <FontAwesomeIcon icon="pencil-alt"/>
                  </Button>

                  <Button title="Delete Artifact"
                          onClick={() => this.setState({showDeleteArtifactDialog: true})}
                          disabled={!deletable}
                          variant="default">
                    <FontAwesomeIcon icon="trash"/>
                  </Button>

                  <Button title="Upload Artifact Pack"
                          onClick={()=>this.setState({showArtifactsUploadDialog: true})}
                          variant="default">
                    <FontAwesomeIcon icon="upload"/>
                  </Button>
                </ButtonGroup>
                <Form inline className="">
                  <InputGroup >
                    <InputGroup.Prepend>
                      <Button variant="outline-default"
                              onClick={() => this.updateSearch("")}
                              size="sm">
                        { this.state.loading ? <FontAwesomeIcon icon="spinner" spin/> :
                          <FontAwesomeIcon icon="broom"/>}
                      </Button>
                    </InputGroup.Prepend>
                    <FormControl size="sm"
                                 ref={(input) => { this.searchInput = input; }}
                                 value={this.state.current_filter}
                                 onChange={(e) => this.updateSearch(e.currentTarget.value)}
                                 placeholder="Search for artifact"
                    />
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
                        type="ARTIFACT_DESCRIPTION"
                        client={{client_id: this.state.selectedDescriptor.name}}
                      /> :
                      <h2 className="no-content">Search for an artifact to view it</h2>
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
