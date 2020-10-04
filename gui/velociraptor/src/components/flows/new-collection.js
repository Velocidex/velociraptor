import './new-collection.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';
import SplitPane from 'react-split-pane';
import BootstrapTable from 'react-bootstrap-table-next';
import VeloReportViewer from "../artifacts/reporting.js";
import Spinner from 'react-bootstrap/Spinner';
import StepWizard from 'react-step-wizard';
import VeloForm from '../forms/form.js';

import VeloAce from '../core/ace.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';


class NewCollectionSelectArtifacts extends React.Component {
    static propTypes = {
        setArtifacts: PropTypes.func,
    };

    state = {
        selectedDescriptor: "",

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        loading: false,

        selected_artifacts: {},
    }

    onSelect = (row, isSelect) => {
        this.setState({selectedDescriptor: row});

        let selected_artifacts = Object.assign({}, this.state.selected_artifacts);
        if (isSelect) {
            selected_artifacts[row.name] = row;
        } else {
            delete selected_artifacts[row.name];
        }
        this.setState({selected_artifacts: selected_artifacts});
    }

    onSelectAll = (isSelect, rows) => {
        let selected_artifacts = Object.assign({}, this.state.selected_artifacts);
        if (isSelect) {
            _.each(rows, (row) => {
                selected_artifacts[row.name] = row;
            });
        } else {
            _.each(rows, (row) => {
                delete selected_artifacts[row.name];
            });
        }
        this.setState({selected_artifacts: selected_artifacts});
    }

    setArtifacts = () => {
        let artifacts = [];
        _.each(this.state.selected_artifacts, (x) => {
            artifacts.push(x);
        });

        this.props.setArtifacts(artifacts);
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
            let items = response.data.items || [];

            for(let i=0; i<items.length; i++) {
                var desc = items[i];
                matchingDescriptors.push(desc);
            };

            this.setState({matchingDescriptors: matchingDescriptors,
                           loading: false});
        });
    }

    render() {
        let columns = [{dataField: "name", text: "", filter: textFilter({
            placeholder: "Search for name...",
        })}];

        let selected = this.state.selectedDescriptor && this.state.selectedDescriptor.name;
        let selectRow = {mode: "checkbox",
                         clickToSelect: true,
                         classes: "row-selected",
                         onSelect: this.onSelect,
                         onSelectAll: this.onSelectAll,
                        };

        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Select artifacts to collect</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <div className="row new-artifact-page">
                  <div className="col-4 new-artifact-search-table">
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
                  <div name="ArtifactInfo" className="col-8 new-artifact-description">
                    { this.loading ? <Spinner
                                       animation="border" role="status">
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
                </div>

              </Modal.Body>
              <Modal.Footer>
                <Button variant="primary"
                        onClick={() => {
                            this.setArtifacts();
                            this.props.lastStep();
                        }}>
                  Launch with Defaults
                </Button>

                <Button variant="primary"
                        onClick={() => {
                            this.setArtifacts();
                            this.props.nextStep();
                        }}>
                  Next
                </Button>
              </Modal.Footer>
            </>
        );
    }
};


class NewCollectionConfigParameters extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        setParameters: PropTypes.func.isRequired,
    };

    state = {
        parameters: {},
    }

    getRequiredParams = () => {
        let parameters = [];
        let seen = {};

        _.each(this.props.artifacts, (artifacts) => {
            _.each(artifacts.parameters, (param) => {
                let name = param.name;

                if(name && !seen[param.name]) {
                    parameters.push(param);
                    seen[param.name] = true;
                };
            });
        });

        return parameters;
    }

    setValue = (name, value) => {
        let parameters = this.state.parameters;
        if (_.isUndefined(value)) {
            delete parameters[name];
        } else {
            parameters[name] = value;
        }

        this.setState({parameters, parameters});

        console.log(parameters);
    }

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Configure artifact parameters</Modal.Title>
              </Modal.Header>

              <Modal.Body className="new-collection-parameter-page">
                { _.map(this.getRequiredParams(), (param, idx) => {
                    let value = this.state.parameters[param.name] || param.default || "";

                    return (
                        <VeloForm param={param} key={idx}
                                  value={value}
                                  setValue={(value) => this.setValue(param.name, value)}/>
                    );
                })};
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.previousStep}>
                  Back
                </Button>
                <Button variant="primary"
                        onClick={() => {
                            this.props.setParameters(this.state.parameters);
                            this.props.nextStep();
                        }}>
                  Launch
                </Button>
              </Modal.Footer>
            </>
        );
    };
}

class NewCollectionResources extends React.Component {
    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Configure resources</Modal.Title>
              </Modal.Header>
              <Modal.Body>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.previousStep}>
                  Back
                </Button>
                <Button variant="primary"
                        onClick={() => {
                            this.props.nextStep();
                        }}>
                  Launch
                </Button>
              </Modal.Footer>
            </>
        );
    }
}

class NewCollectionRequest extends React.Component {
    static propTypes = {
        request: PropTypes.object,
    }

    render() {
        let serialized =  JSON.stringify(this.props.request, null, 2);
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Examine request</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloAce text={serialized} options={{readOnly: true}} />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.previousStep}>
                  Back
                </Button>
                <Button variant="primary"
                        onClick={() => {
                            this.props.nextStep();
                        }}>
                  Launch
                </Button>
              </Modal.Footer>
            </>
        );
    }
}


export default class NewCollectionWizard extends React.Component {
    static propTypes = {
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    }

    state = {
        artifacts: [],
        parameters: {env: []},
        resources: {},
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        console.log(params);
        let new_parameters = {env: []};
        for (var k in params) {
            if (params.hasOwnProperty(k)) {
                new_parameters.env.push({key: k, value: params[k]});
            }
        }
        this.setState({parameters: new_parameters});
    }

    setResources = (resources) => {
        this.setState({resources: resources});
    }

    prepareRequest = () => {
        let artifacts = [];
        _.each(this.state.artifacts, (item) => {
            artifacts.push(item.name);
        });

        return {
            artifacts: artifacts,
            parameters: this.state.parameters,
        };
    }

    render() {
        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <StepWizard>
                <NewCollectionSelectArtifacts
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  setParameters={this.setParameters}
                  artifacts={this.state.artifacts}/>

                <NewCollectionResources
                  setResources={this.setResources}
                />

              <NewCollectionRequest
                request={this.prepareRequest()}
              />

                </StepWizard>;
            </Modal>
        );
    }
}
