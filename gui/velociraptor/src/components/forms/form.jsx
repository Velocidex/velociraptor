import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.jsx';
import _ from 'lodash';
import {CancelToken} from 'axios';
import DateTimePicker from '../widgets/datetime.jsx';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import RegEx from './regex.jsx';
import RegExArray from './regex_array.jsx';
import UploadFileForm from './upload.jsx';
import YaraEditor from './yara.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import Alert from 'react-bootstrap/Alert';
import CSVForm from './csv.jsx';
import JSONArrayForm from './json_array.jsx';
import Select from 'react-select';
import T from '../i8n/i8n.jsx';
import { JSONparse } from '../utils/json_parse.jsx';
import { parseCSV, serializeCSV } from '../utils/csv.jsx';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import "./validated.css";
import "./forms.css";

// Should match the code in services/launcher/compiler.go
const boolRegex = RegExp('^(Y|TRUE|YES|OK)$', "i");

export default class VeloForm extends React.Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.any,
        setValue: PropTypes.func.isRequired,
    };

    state = {

        // A Date() object that is parsed from value in local time.
        timestamp: null,
        multichoices: undefined,
        artifact_loading: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.maybeFetchArtifacts(this.props.param.artifact_type);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        this.maybeFetchArtifacts(this.props.param.artifact_type);
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    maybeFetchArtifacts(artifact_type) {
        if (!artifact_type || !this.props.param ||
            this.props.param.type !== "artifactset") {
            return;
        }

        // Only fetch the list once.
        if (this.state.multichoices !== undefined) {
            return;
        }

        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        let fields = {name: true};
        if (this.props.param.sources) {
            fields["sources"] = true;
        };

        // Load all artifacts, but only keep the ones that match the
        // specified type
        api.post("v1/GetArtifacts",
                {
                    search_term: "empty:true",
                    type: artifact_type,

                    fields: fields,

                    // This might be too many to fetch at once but we
                    // are still fast enough for now.
                    number_of_results: 1000,
                },

                this.source.token).then((response) => {
                    if (response.cancel) return;

                    let artifacts = {};
                    let items = response.data.items || [];
                    let data = parseCSV(this.props.value);
                    let checkedArtifacts = _.map(data.data, function(item, idx) {
                        return item.Artifact;
                    });

                    for (let i=0; i < items.length; i++) {
                        let desc = items[i];

                        let sources = desc.sources || [];
                        let selectable_names = [];
                        let descriptions = {};
                        let unnamed_sources = 0;

                        // Gather the list of possible sources so the user
                        // can select them individually.  If there are
                        // unnamed sources, we'll save them for the end
                        // since the results will have been combined and
                        // available under the unqualified artifact name.
                        for (let j=0; j < sources.length; j++) {
                            if (sources[j].name) {
                                let name = desc.name + "/" + sources[j].name;

                                selectable_names.push(name);
                                if (sources[j].description) {
                                    descriptions[name] = sources[j].description;
                                } else {
                                    descriptions[name] = desc.description;
                                }
                            } else {
                                unnamed_sources++;
                            }
                        }

                        if(_.isEmpty(sources)) {
                            selectable_names.push(desc.name);
                        }

                        // If there is a mix of unnamed sources and named
                        // sources, put the bare artifact name first so the
                        // UI will be visually consistent.
                        if (unnamed_sources) {
                            selectable_names.unshift(desc.name);
                            descriptions[desc.name] = desc.description;
                        }

                        for (let j=0; j < selectable_names.length; j++) {
                            let name = selectable_names[j];
                            artifacts[name] = {
                                'description' : descriptions[name],
                                'enabled' : false,
                            };
                            if (checkedArtifacts.indexOf(name) !== -1) {
                                artifacts[name].enabled = true;
                            }
                        }
                    };

                    // Keep artifacts that may have been temporarily removed
                    let unavailableArtifacts = _.without(checkedArtifacts, Object.keys(artifacts));

                    this.setState({
                        multichoices: artifacts,
                        artifact_loading: false,
                        unavailableArtifacts: unavailableArtifacts,
                    });
                });
    }

    updateMulti(newChoices) {
        var choices = [];
        for (var name in newChoices) {
            if (newChoices[name].enabled) {
                choices.push(name);
            }
        }
        choices.concat(this.state.unavailableArtifacts);

        // Assemble into a simple CSV so it's trivial to parse as a parameter
        this.props.setValue(serializeCSV(_.map(choices, function(item, idx) {
                return [item];
        }), ['Artifact']));
    }

    setMulti(k, e) {
        let value = e.currentTarget.checked;
        this.setState((state) => {
            let choices = { ...state.multichoices };
            choices[k].enabled = value;
            this.updateMulti(choices);
            return { multichoices: choices };
        });
    }

    setValueWithValidator = (value, validator)=>{
        let invalid = false;
        if (validator) {
            try {
                // Not multi line match because the regex would
                // typically anchor to the start and end of the entire
                // string.
                const regexp = new RegExp(validator, 'is');
                if (_.isEmpty(value)) {
                    invalid = false;
                } else if (regexp.test(value)) {
                    invalid = false;
                } else {
                    invalid = true;
                }

                this.setState({invalid: invalid});
            } catch(e){
                console.log("setValueWithValidator", e);
            }
        }
        this.props.setValue(value);
    }

    render() {
        let param = this.props.param || {};
        let name = param.friendly_name || param.name;

        switch(param.type) {
        case "hidden":
            return <></>;

        case "json_array": {
            return <JSONArrayForm
                     param={this.props.param}
                     value={this.props.value}
                     setValue={this.props.setValue}/>;
        }

        case "csv": {
            return <CSVForm
                     param={this.props.param}
                     value={this.props.value}
                     setValue={this.props.setValue}/>;
        }

        case "regex":
            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                    <Col sm="8">
                      <RegEx
                        value={this.props.value}
                        setValue={this.props.setValue}
                      />
                  </Col>
                </Form.Group>
            );

        case "regex_array":
            return (
                  <Form.Group as={Row}  className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description || ""}>
                      {name}
                    </ToolTip>
                  </Form.Label>
                    <Col sm="8">
                      <RegExArray
                        value={this.props.value}
                        setValue={this.props.setValue}
                      />
                  </Col>
                </Form.Group>
            );

        case "yara":
            return (
                  <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                    <Col sm="8">
                      <YaraEditor
                        value={this.props.value}
                        setValue={this.props.setValue}
                      />
                  </Col>
                </Form.Group>
            );

        case "timestamp": {
            // value prop is always a string in ISO format in UTC timezone.
            let date = this.props.value;

            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <DateTimePicker
                      onChange={(value) => {
                          // Clear the prop value if the value is not
                          // a real date.
                          if (_.isDate(value)) {
                              this.props.setValue(value.toISOString());
                          } else if (_.isString(value)) {
                              this.props.setValue(value);
                          } else {
                              this.props.setValue(undefined);
                          }
                      }}
                      value={date}
                    />
                  </Col>
                </Form.Group>
            );
        }

        case "choices":
            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="select"
                                  value={this.props.value}
                                  onChange={(e) => {
                                      this.props.setValue(e.currentTarget.value);
                                  }}>
                      {_.map(this.props.param.choices || [], function(item, idx) {
                          return <option key={idx}>{item}</option>;
                      })}
                    </Form.Control>
                  </Col>
                </Form.Group>
            );

        case "multichoice": {
            let options = [];
            _.each(this.props.param.choices, x=>{
                options.push({value: x, label: x});
            });

            let defaults = _.map(JSONparse(this.props.value, []),
                                 x=>{return {value: x, label: x};});

            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <ButtonGroup className="full-width multichoice-select">
                      <Button variant="default"
                              onClick={x=>{
                                  this.props.setValue(JSON.stringify(
                                      this.props.param.choices));
                              }}>
                        <FontAwesomeIcon icon="border-all"/>
                      </Button>
                        <Select
                          placeholder={T("Choose one or more items")}
                          className="velo"
                          classNamePrefix="velo"
                          closeMenuOnSelect={false}
                          isMulti
                          defaultValue={defaults}
                          value={defaults}
                          onChange={e=>{
                              let data = _.map(e, x=>x.value);
                              this.props.setValue(JSON.stringify(data));
                          }}
                          options={options}
                        />
                    </ButtonGroup>
                  </Col>
                </Form.Group>
            );
        }
        case "artifactset": {
            // No artifacts means we haven't loaded yet.  If there are
            // truly no artifacts, we've got bigger problems.
            if (this.state.multichoices === undefined ||
                this.state.artifact_loading) {
              return <></>;
            }
            if (Object.keys(this.state.multichoices).length === 0) {
                return (
                  <Form.Group as={Row} className="velo-form">
                    <Alert variant="danger">
                      Warning: No artifacts found for type {
                          this.props.param.artifact_type
                      }.
                    </Alert>
                  </Form.Group>
                );
            }

            let a_options = [];
            let a_defaults = [];

            _.each(this.state.multichoices, (v, k)=>{
                a_options.push({value: k, label: k});
                if(v.enabled) {
                    a_defaults.push({value: k, label: k});
                }
            });

            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <Select
                      placeholder={T("Choose one or more items")}
                      className="velo"
                      classNamePrefix="velo"
                      closeMenuOnSelect={false}
                      isMulti
                      defaultValue={a_defaults}
                      onChange={e=>{
                          let data = _.map(e, x=>{
                              return {Artifact: x.value};
                          });
                          // artifact sets require csv style output.
                          this.props.setValue(serializeCSV(data, ["Artifact"]));
                      }}
                      options={a_options}
                      />
                  </Col>
                </Form.Group>
            );
        }
        case "bool":
            return (
                <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="switch"
                      label={param.description}
                      onChange={(e) => {
                          if (e.currentTarget.checked) {
                              this.props.setValue("Y");
                          } else {
                              this.props.setValue("N");
                          }
                      }}
                      checked={boolRegex.test(this.props.value)}
                      value={this.props.value} />
                  </Col>
                </Form.Group>
            );

        case "upload_file":
        case "upload":
            return <UploadFileForm
                     param={this.props.param}
                     value={this.props.value}
                     setValue={this.props.setValue}
                   />;
        default:
            return (
                  <Form.Group as={Row} className="velo-form">
                  <Form.Label column sm="3">
                    <ToolTip tooltip={param.description}>
                      <div>
                        {name}
                      </div>
                    </ToolTip>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  placeholder={this.props.param.description}
                                  className={ this.state.invalid && 'invalid' }
                                  rows={1}
                                  onChange={(e) => {
                                      this.setValueWithValidator(
                                          e.currentTarget.value,
                                          this.props.param.validating_regex);
                                  }}
                                  value={this.props.value} />
                  </Col>
                </Form.Group>
            );
        }
    };
}
