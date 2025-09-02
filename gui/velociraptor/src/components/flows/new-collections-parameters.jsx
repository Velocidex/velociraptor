import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Autosuggest from 'react-autosuggest';

import VeloForm from '../forms/form.jsx';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Form from 'react-bootstrap/Form';
import ToolTip from '../widgets/tooltip.jsx';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Accordion from 'react-bootstrap/Accordion';


class ResourceControl extends React.Component {
    static propTypes = {
        parameters: PropTypes.object,
        artifact: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    }

    state = {
        invalid: false,
        visible: false,
    }

    render() {
        if (!this.state.visible) {
            return <Button onClick={()=>this.setState(
                {visible: !this.state.visible})}>
                     <FontAwesomeIcon icon="wrench"/>
                   </Button>;
        }

        let params = this.props.parameters[this.props.artifact] || {};
        let max_batch_wait = params.max_batch_wait;
        let max_batch_rows = params.max_batch_rows;
        let cpu_limit = params.cpu_limit;
        let timeout = params.timeout;

        let params_batch_wait = {validating_regex: "\\d+",
                                 description: "Default",
                                 name: "max_batch_wait"};
        let params_batch_rows = {validating_regex: "\\d+",
                                 description: "Default",
                                 name: "max_batch_rows"};
        let params_cpu_limit = {validating_regex: "\\d+",
                                description: "100%",
                                name: "cpu_limit"};
        let params_timeout = {validating_regex: "\\d+",
                              description: "100%",
                              name: "timeout"};

        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">
                {T("Configuration")}
              </Form.Label>
              <Col sm="8">
                <VeloForm param={params_cpu_limit}
                          value={cpu_limit}
                          setValue={x=>this.props.setValue("cpu_limit", x)}
                />

                <VeloForm param={params_batch_wait}
                          value={max_batch_wait}
                          setValue={x=>this.props.setValue("max_batch_wait", x)}
                />

                <VeloForm param={params_batch_rows}
                          value={max_batch_rows}
                          setValue={x=>this.props.setValue("max_batch_rows", x)}
                />
                <VeloForm param={params_timeout}
                          value={timeout}
                          setValue={x=>this.props.setValue("timeout", x)}
                />

              </Col>
            </Form.Group>
        );
    }
}

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

export class NewCollectionConfigParametersForm extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func,
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        configureResourceControl: PropTypes.bool,
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
        if(this.props.configureResourceControl) {
            results.push(<ResourceControl
              key="X"
              parameters={this.props.parameters}
              artifact={artifact.name}
              setValue={(param_name, value) => this.setValue(
                  artifact.name, param_name, value)}/>);
        };
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
        return (
            <>
              <Accordion>
                { _.map(this.props.artifacts, (x, idx)=>{
                    return <Accordion.Item eventKey={idx} key={idx}>
                                   <Accordion.Header>
                                     <ButtonGroup>
                                       <ToolTip tooltip={T("Configure")}>
                                         <div className="accordion-icon"
                                              tabIndex={idx}
                                              role="button">
                                           <FontAwesomeIcon icon="wrench"/>
                                         </div>
                                       </ToolTip>
                                       { this.props.setArtifacts &&
                                         <ToolTip tooltip={T("Remove")}>
                                           <div className="accordion-icon"
                                                role="button" tabIndex={idx}
                                                onClick={
                                                    () => this.props.setArtifacts(
                                                        remove_artifact(
                                                            this.props.artifacts,
                                                            x.name))} >
                                             <FontAwesomeIcon icon="trash"/>
                                           </div>
                                         </ToolTip>
                                       }
                                     </ButtonGroup>
                                     {x.name}
                                   </Accordion.Header>
                                   <Accordion.Body>
                                     {this.artifactParameterRenderer(x)}
                                   </Accordion.Body>
                                 </Accordion.Item>;
                })}
              </Accordion>
            </>
        );
    };
}


export default class NewCollectionConfigParameters extends NewCollectionConfigParametersForm {
    static propTypes = {
        artifacts: PropTypes.array,
        setArtifacts: PropTypes.func,
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
        configureResourceControl: PropTypes.bool,
    };

    render() {
        return <>
                 <Modal.Header closeButton>
                   <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
                 </Modal.Header>
                 <Modal.Body className="new-collection-parameter-page selectable">
                   { super.render() }
                 </Modal.Body>
                 <Modal.Footer>
                   { this.props.paginator.makePaginator({
                       props: this.props,
                   }) }
                 </Modal.Footer>
               </>;

    }
}
