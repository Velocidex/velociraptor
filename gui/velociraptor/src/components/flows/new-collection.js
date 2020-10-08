import './new-collection.css';

import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

import Button from 'react-bootstrap/Button';
import Modal from 'react-bootstrap/Modal';
import filterFactory, { textFilter } from 'react-bootstrap-table2-filter';
import SplitPane from 'react-split-pane';
import BootstrapTable from 'react-bootstrap-table-next';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import VeloReportViewer from "../artifacts/reporting.js";

import Spinner from 'react-bootstrap/Spinner';
import Col from 'react-bootstrap/Col';

import StepWizard from 'react-step-wizard';
import VeloForm from '../forms/form.js';

import ValidatedInteger from "../forms/validated_int.js";

import VeloAce from '../core/ace.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';


class PaginationBuilder {
    PaginationSteps = ["Select Artifacts", "Configure Parameters",
                       "Specify Resorces", "Review", "Launch"];

    constructor(name, title, shouldFocused) {
        this.title = title;
        this.name = name;
        if (shouldFocused) {
            this.shouldFocused = shouldFocused;
        }
    }

    title = ""
    name = ""

    shouldFocused = (isFocused, step) => isFocused;

    // A common function to create the modal paginator between wizard
    // pages.
    // onBlur is a function that will be called when we leave the current page.
    // If isFocused is true, the current page is focused and all over navigators are disabled.
    makePaginator = (spec) => {
        let {props, onBlur, isFocused} = spec;
        return <Col md="12">
                 <Pagination>
                   { _.map(this.PaginationSteps, (step, i) => {
                       let idx = i;
                       if (step === this.name) {
                           return <Pagination.Item
                                    active key={idx}>
                                    {step}
                                  </Pagination.Item>;
                       };
                       return <Pagination.Item
                                onClick={() => {
                                    if (onBlur) {onBlur();};
                                    props.goToStep(idx + 1);
                                }}
                                disabled={this.shouldFocused(isFocused, step)}
                                key={idx}>
                                {step}
                              </Pagination.Item>;
                   })}
                 </Pagination>
               </Col>;
    }
}

class NewCollectionSelectArtifacts extends React.Component {
    static propTypes = {
        // A list of artifact descriptors that are pre-selected.
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func,
        paginator: PropTypes.object,
    };

    componentDidMount = () => {
         this.initFromParent();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEmpty(this.props.artifacts) && !this.state.initialized_from_parent) {
            this.initFromParent();
        }
    }


    // Fetch descriptors for artifacts that are initialized from props.
    initFromParent = () => {
        let init_artifacts = this.props.artifacts;
        if (!_.isEmpty(init_artifacts)) {
            this.setState({
                initialized_from_parent: true,
                matchingDescriptors: [...init_artifacts],
                selected_artifacts: [...init_artifacts],
                selectedDescriptor: init_artifacts[0]});
        }
    }

    state = {
        selectedDescriptor: "",

        // A list of descriptors that match the search term.
        matchingDescriptors: [],

        loading: false,

        selected_artifacts: {},

        initialized_from_parent: false,
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
            let items = response.data.items || [];
            this.setState({matchingDescriptors: items, loading: false});
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
                <Modal.Title>{ this.props.paginator.title }</Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <div className="row new-artifact-page">
                  <div className="col-4 new-artifact-search-table">
                    {
                      <BootstrapTable
                        remote={ { filter: true } }
                        filter={ filterFactory() }
                        keyField="name"
                        data={this.state.matchingDescriptors}
                        columns={columns}
                        selectRow={ selectRow }
                        onTableChange={ this.updateSearch }
                      />
                    }
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
                      />
                    }
                  </div>
                </div>

              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    onBlur: this.setArtifacts,
                    isFocused: _.isEmpty(this.state.selected_artifacts),
                }) }
              </Modal.Footer>
            </>
        );
    }
};

class NewCollectionConfigParameters extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        artifacts: PropTypes.array,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
    };

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let request = this.props.request;
        if (request && !this.state.initialized_from_parent) {
            let params = request.parameters && request.parameters.env;
            let params_obj = {};
            _.each(params, (item) => {
                params_obj[item.key] = item.value;
            });
            this.setState({parameters: params_obj,
                           initialized_from_parent: true});
        }
    }

    state = {
        // A K/V mapping that is maintained by the component.
        parameters: {},
        initialized_from_parent: false,
    }

    setValue = (name, value) => {
        let parameters = this.state.parameters;
        if (_.isUndefined(value)) {
            delete parameters[name];
        } else {
            parameters[name] = value;
        }

        this.setState({parameters, parameters});
    }

    render() {
        const expandRow = {
            expandHeaderColumnRenderer: ({ isAnyExpands }) => {
                if (isAnyExpands) {
                    return <b>-</b>;
                }
                return <b>+</b>;
            },
            expandColumnRenderer: ({ expanded }) => {
                if (expanded) {
                    return (
                        <b>-</b>
                    );
                }
                return (
                    <b><FontAwesomeIcon icon="wrench"/></b>
                );
            },
            showExpandColumn: true,
            renderer: artifact => {
                return _.map(artifact.parameters || [], (param, idx) => {
                    let value = this.state.parameters[param.name] || param.default || "";

                    return (
                        <VeloForm param={param} key={idx}
                                  value={value}
                                  setValue={(value) => this.setValue(param.name, value)}/>
                    );
                });
            }
        };

        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Configure artifact parameters</Modal.Title>
              </Modal.Header>

              <Modal.Body className="new-collection-parameter-page">
                { !_.isEmpty(this.props.artifacts) &&
                  <BootstrapTable
                    keyField="name"
                    expandRow={ expandRow }
                    columns={[{dataField: "name", text: "Artifact"},
                              {dataField: "parameter", text: "", hidden: true}]}
                    data={this.props.artifacts} />
                }
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    onBlur: () => this.props.setParameters(this.state.parameters),
                }) }
              </Modal.Footer>
            </>
        );
    };
}

class NewCollectionResources extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        setResources: PropTypes.func,
        paginator: PropTypes.object,
    }

    state = {
        initialized_from_parent: false,
        ops_per_second: null,
        timeout: null,
        max_rows: null,
        max_mbytes: null,
    }

    componentDidMount = () => {
        this.initFromParent();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        this.initFromParent();
    }

    initFromParent = () => {
        let request = this.props.request;
        if (request && !this.state.initialized_from_parent) {
            this.setState({
                initialized_from_parent: true,
                ops_per_second: request.ops_per_second,
                timeout: parseInt(request.timeout),
                max_rows: request.max_rows,
                max_mbytes: (request.max_upload_bytes || 0) / 1024 / 1024,
            });
        }
    }

    isInvalid = () => {
        return this.state.invalid_1 || this.state.invalid_2 ||
            this.state.invalid_3 || this.state.invalid_4;
    }

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Collection - Configure resources</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Ops/Sec</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        defaultValue={this.state.ops_per_second}
                        setInvalid={value => this.setState({invalid_1: value})}
                        setValue={value => this.setState({ops_per_second: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Execution Time in Seconds</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="600"
                        defaultValue={this.state.timeout}
                        setInvalid={value => this.setState({invalid_2: value})}
                        setValue={value => this.setState({timeout: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Rows</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        defaultValue={this.state.max_rows}
                        setInvalid={value => this.setState({invalid_3: value})}
                        setValue={value => this.setState({max_rows: value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Max Mb Uploaded</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger
                        placeholder="Unlimited"
                        defaultValue={this.state.max_mbytes}
                        setInvalid={value => this.setState({invalid_4: value})}
                        setValue={value => this.setState({max_mbytes: value})} />
                    </Col>
                  </Form.Group>
                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    onBlur: () => this.props.setResources(this.state),
                    isFocused: this.isInvalid(),
                }) }
              </Modal.Footer>
            </>
        );
    }
}

class NewCollectionRequest extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        paginator: PropTypes.object,
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
                { this.props.paginator.makePaginator({
                    props: this.props,
                }) }
              </Modal.Footer>
            </>
        );
    }
}


class NewCollectionLaunch extends React.Component {
    static propTypes = {
        launch: PropTypes.func,
        isActive: PropTypes.bool,
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.isActive && !prevProps.isActive) {
            this.props.launch();
        }
    }

    render() {
        return <></>;
    }
}

class NewCollectionWizard extends React.Component {
    static propTypes = {
        baseFlow: PropTypes.object,
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    }

    componentDidMount = () => {
        this.initializeFromBaseFlow();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!this.state.original_flow && this.props.baseFlow) {
            this.initializeFromBaseFlow();
        }
    }

    initializeFromBaseFlow = () => {
        let request = this.props.baseFlow && this.props.baseFlow.request;
        if (!request || !request.artifacts) {
            return;
        }

        this.setState({
            original_flow: this.props.baseFlow,
            initialized_from_parent: true,
        });

        // Resolve the artifacts from the request into a list of descriptors.
        api.get("api/v1/GetArtifacts", {names: request.artifacts}).
            then((response) => {
                if (response && response.data &&
                    response.data.items && response.data.items.length) {

                    this.setState({
                        artifacts: [...response.data.items],
                        parameters: request.parameters,
                    });
                }});
    }

    state = {
        original_flow: null,

        // A list of artifact descriptors we have selected so far.
        artifacts: [],

        // A key/value mapping of edited parameters by the user.
        parameters: {env: []},

        resources: {},

        initialized_from_parent: false,
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
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

    // Let our caller know the artifact request we created.
    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    prepareRequest = () => {
        let artifacts = [];
        _.each(this.state.artifacts, (item) => {
            artifacts.push(item.name);
        });

        let result = {
            artifacts: artifacts,
            parameters: this.state.parameters,
        };

        if (this.state.resources.ops_per_second) {
            result.ops_per_second = this.state.resources.ops_per_second;
        }

        if (this.state.resources.timeout) {
            result.timeout = this.state.resources.timeout;
        }

        if (this.state.resources.max_rows) {
            result.max_rows = this.state.resources.max_rows;
        }

        if (this.state.resources.max_mbytes) {
            result.max_upload_bytes = this.state.resources.max_mbytes * 1024 * 1024;
        }

        return result;
    }

    render() {
        let paginator = new PaginationBuilder();
        let request = this.state.original_flow && this.state.original_flow.request;

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <StepWizard>
                <NewCollectionSelectArtifacts
                  artifacts={this.state.artifacts}
                  paginator={new PaginationBuilder("Select Artifacts",
                                                   "New Collection: Select Artifacts to collect")}
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  setParameters={this.setParameters}
                  artifacts={this.state.artifacts}
                  paginator={new PaginationBuilder("Configure Parameters",
                                                   "New Collection: Configure Parameters")}
                  request={request}/>

                <NewCollectionResources
                  request={request}
                  paginator={new PaginationBuilder("Specify Resorces",
                                                   "New Collection: Specify Resources")}
                  setResources={this.setResources} />

                <NewCollectionRequest
                  paginator={new PaginationBuilder("Review",
                                                   "New Collection: Review request")}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  launch={this.launch} />

              </StepWizard>
            </Modal>
        );
    }
}


export {
    NewCollectionWizard as default,
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionResources,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
 }
