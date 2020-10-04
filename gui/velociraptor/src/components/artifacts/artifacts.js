import './artifacts.css';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';

import React from 'react';
import PropTypes from 'prop-types';

import classNames from "classnames";
import api from '../core/api-service.js';

import VeloReportViewer from "../artifacts/reporting.js";

import _ from 'lodash';

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import Spinner from 'react-bootstrap/Spinner';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import BootstrapTable from 'react-bootstrap-table-next';

import { withRouter }  from "react-router-dom";

import SplitPane from 'react-split-pane';

const resizerStyle = {
//    width: "25px",
};

class ArtifactInspector extends React.Component {
    static propTypes = {

    };

    state = {
        selectedDescriptor: "",

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        // A cache of all known descriptors.
        descriptors: {},

        timeout: 0,

        loading: false,
    }

    componentDidMount = () => {
        let artifact_name = this.props.match && this.props.match.params &&
            this.props.match.params.artifact;

        if (!artifact_name) {
            return;
        }
        this.setState({selectedDescriptor: {name: artifact_name}});
    }

    updateSearch = (type, filters) => {
        let value = filters && filters.filters && filters.filters.name &&
            filters.filters.name.filterVal;
        if (!value) {
            return;
        }

        this.setState({loading: true});
        api.get("api/v1/GetArtifacts", {search_term: value}).then((response) => {
            let matchingDescriptors = [];
            let descriptors = this.state.descriptors;
            let items = response.data.items || [];

            for(let i=0; i<items.length; i++) {
                var desc = items[i];
                descriptors[desc.name] = desc;
                matchingDescriptors.push(desc);
            };

            this.setState({matchingDescriptors: matchingDescriptors,
                           loading: false,
                           descriptors: descriptors});
        });
    }

    onSelect = (row) => {
        this.setState({selectedDescriptor: row});
        this.props.history.push("/artifacts/" + row.name);
    }

    render() {
        let columns = [{dataField: "name", text: "", filter: textFilter({
            placeholder: "Search for name...",
        })}];

        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let selectRow = {mode: "radio",
                         clickToSelect: true,
                         hideSelectColumn: true,
                         classes: "row-selected",
                         selected: [selected],
                         onSelect: this.onSelect};

        return (
            <div className="full-width-height">
              <Navbar className="toolbar  row">
                  <ButtonGroup>
                    <Button title="Add an Artifact"
                            onClick={this.updateArtifactDefinitions}
                            variant="default">
                      <FontAwesomeIcon icon="plus"/>
                    </Button>

                    <Button title="Edit an Artifact"
                            onClick={() => {
                                this.updateArtifactDefinitions(this.state.selectedDescriptor);
                            }}
                            variant="default">
                      <FontAwesomeIcon icon="pencil-alt"/>
                    </Button>

                    <Button title="Delete Artifact"
                            onClick={this.deleteArtifactDefinitions}
                            variant="default">
                      <FontAwesomeIcon icon="trash"/>
                    </Button>

                    <Button title="Upload Artifact Pack"
                            onClick={this.uploadArtifacts}
                            variant="default">
                      <FontAwesomeIcon icon="upload"/>
                    </Button>

                  </ButtonGroup>
                </Navbar>
              <div className="artifact-search-panel">
                <SplitPane
                  split="vertical"
                  defaultSize="30%"
                  resizerStyle={resizerStyle}>
                  <div className="artifact-search-table">
                    <BootstrapTable
                      remote={ { filter: true } }
                      filter={ filterFactory() }
                      keyField="name"
                      data={this.state.matchingDescriptors}
                      columns={columns}
                      selectRow={ selectRow }
                      onTableChange={ this.updateSearch }
                      />
                  </div>
                  <div name="ArtifactInfo" className="artifact-search-report">
                    { this.loading ? <Spinner animation="border" role="status">
                      <span className="sr-only">Loading...</span>
                      </Spinner> :

                      this.state.selectedDescriptor &&
                      <VeloReportViewer
                        artifact={this.state.selectedDescriptor.name}
                        type="ARTIFACT_DESCRIPTION"
                        client={{client_id: this.state.selectedDescriptor.name}}
                      />
                    }
                  </div>
                </SplitPane>
              </div>
            </div>
        );
    }
};

export default withRouter(ArtifactInspector);
