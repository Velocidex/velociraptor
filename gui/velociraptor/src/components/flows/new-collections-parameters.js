import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Autosuggest from 'react-autosuggest';

import VeloForm from '../forms/form.js';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.js';
import BootstrapTable from 'react-bootstrap-table-next';

class ParameterSuggestion extends React.Component {
    static propTypes = {
        parameters: PropTypes.array,
        setFilter: PropTypes.func.isRequired,
    }

    state = {
        suggestions: [],
        value: "",
    }

    componentDidMount = () => {
        this.props.setFilter("");
    }

    onChange = (event, { newValue }) => {
        this.props.setFilter(newValue.toLowerCase());
        this.setState({
            value: newValue
        });
    };

    getSuggestionValue = suggestion => suggestion.name;

    getSuggestions = value => {
        const inputValue = value.trim().toLowerCase();
        const inputLength = inputValue.length;

        return inputLength === 0 ? [] : this.props.parameters.filter(x =>{
            try {
                return x.name.toLowerCase().search(inputValue) >= 0;
            } catch(e) {
                return value;
            };
        });
    };

    onSuggestionsFetchRequested = ({ value }) => {
        this.setState({
            suggestions: this.getSuggestions(value)
        });
    };

    onSuggestionsClearRequested = () => {
        this.setState({
            suggestions: []
        });
    };

    renderSuggestion = suggestion => (
        <div key={suggestion.name}>
          {suggestion.name}
        </div>
    );

    render() {
        const inputProps = {
            placeholder: 'Filter artifact parameter name',
            value: this.state.value,
            onChange: this.onChange,
        };
        return (
            <Row key="Autosuggest"
                 name="AutoSuggest" className="param-form-autosuggest">
              <Col sm="3">
                <Autosuggest
                  suggestions={this.state.suggestions}
                  onSuggestionsFetchRequested={this.onSuggestionsFetchRequested}
                  onSuggestionsClearRequested={this.onSuggestionsClearRequested}
                  getSuggestionValue={this.getSuggestionValue}
                  renderSuggestion={this.renderSuggestion}
                  inputProps={inputProps}
                />
              </Col>
              <Col sm="8">
              </Col>
            </Row>

        );
    }
}



// Remove the named artifact from the list of artifacts
const remove_artifact = (artifacts, name) => {
    return _.filter(artifacts, (x) => x.name !== name);
};

export default class NewCollectionConfigParameters extends React.Component {
    static propTypes = {
        request: PropTypes.object,
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func.isRequired,
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
    };

    setValue = (artifact, name, value) => {
        let parameters = this.props.parameters;
        let artifact_parameters = parameters[artifact] || {};
        artifact_parameters[name] = value;
        parameters[artifact] = artifact_parameters;
        this.props.setParameters(parameters);
    }

   removeArtifact = (name) => {
        this.props.setArtifacts(remove_artifact(this.props.artifacts, name));
    }

    state = {
        filters: {},
    }

    artifactParameterRenderer = artifact => {
        let artifact_parameters = (
            this.props.parameters &&
                this.props.parameters[artifact.name]) || {};

        let suggestions = [];
        let form_parameters = _.map(artifact.parameters, (param, idx) => {
            suggestions.push({name: param.name, key: idx});

            let filter = this.state.filters[artifact.name] || "";

            try {
                if (filter && param.name.toLowerCase().search(filter) < 0 ) {
                    return <div key={idx}></div>;
                }
            } catch(e) {};

            let value = artifact_parameters[param.name];
            // Only set default value if the parameter is not
            // defined. If it is an empty string then so be
            // it.
            if (_.isUndefined(value)) {
                value = param.default || "";
            }

            return (
                <VeloForm param={param} key={idx} name={idx}
                          value={value}
                          setValue={(value) => this.setValue(
                              artifact.name,
                              param.name,
                              value)}/>
               );
        });

        let results = [];
        if(suggestions.length > 6) {
            results.push(
                <ParameterSuggestion key="Autosuggest" name="Autosuggest"
                        parameters={suggestions}
                        setFilter={x=>{
                            let filters = this.state.filters;
                            filters[artifact.name] = x;
                            this.setState({filters: filters});
                        }} />
            );
        }

        results = results.concat(form_parameters);
        return results;

    }

    render() {
        const expandRow = {
            expandHeaderColumnRenderer: ({ isAnyExpands }) => {
                if (isAnyExpands) {
                    return <b>-</b>;
                }
                return <b>+</b>;
            },
            expandColumnRenderer: ({ expanded, rowKey }) => {
                if (expanded) {
                    return (
                        <b key={rowKey}>-</b>
                    );
                }
                return (<ButtonGroup>
                          <Button
                            className="btn-tooltip"
                            data-position="right"
                            data-tooltip={T("Configure")}
                            variant="outline-default">
                            <FontAwesomeIcon icon="wrench"/></Button>
                          <Button variant="outline-default"
                                  className="btn-tooltip"
                                  data-position="right"
                                  data-tooltip={T("Remove")}
                                  onClick={
                              () => this.props.setArtifacts(remove_artifact(
                                  this.props.artifacts, rowKey))} >
                            <FontAwesomeIcon icon="trash"/>
                          </Button>
                        </ButtonGroup>
                );
            },
            showExpandColumn: true,
            renderer: this.artifactParameterRenderer,
        };

        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
              </Modal.Header>

              <Modal.Body className="new-collection-parameter-page selectable">

                { !_.isEmpty(this.props.artifacts) ?
                  <BootstrapTable
                    keyField="name"
                    expandRow={ expandRow }
                    columns={[{dataField: "name", text: T("Artifact")},
                              {dataField: "parameter", text: "", hidden: true}]}
                    data={this.props.artifacts} /> :
                 <div className="no-content">
                   {T("No artifacts configured. Please add some artifacts to collect")}
                 </div>
                }
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                }) }
              </Modal.Footer>
            </>
        );
    };
}
