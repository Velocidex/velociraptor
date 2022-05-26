import React from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.js';
import _ from 'lodash';
import axios from 'axios';
import DateTimePicker from 'react-datetime-picker';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Button from 'react-bootstrap/Button';
import RegEx from './regex.js';
import UploadFileForm from './upload.js';
import YaraEditor from './yara.js';
import Tooltip from 'react-bootstrap/Tooltip';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Alert from 'react-bootstrap/Alert';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import BootstrapTable from 'react-bootstrap-table-next';
import cellEditFactory, { Type } from 'react-bootstrap-table2-editor';
import { parseCSV, serializeCSV } from '../utils/csv.js';
import "./validated.css";

const numberRegex = RegExp("^[0-9]+$");

// Returns a date object in local timestamp which represents the UTC
// date. This is needed because the date selector widget expects to
// work in local time.
function localTimeFromUTCTime(date) {
    let msSinceEpoch = date.getTime();
    let tzoffset = (new Date()).getTimezoneOffset();
    return new Date(msSinceEpoch + tzoffset * 60000);
}

function utcTimeFromLocalTime(date) {
    let msSinceEpoch = date.getTime();
    let tzoffset = (new Date()).getTimezoneOffset();
    return new Date(msSinceEpoch - tzoffset * 60000);
}

function convertToDate(x) {
    // Allow the value to be specified in a number of ways.
    if (_.isNumber(x) || numberRegex.test(x)) {
        try {
            return new Date(parseInt(x) * 1000);
        } catch(e) {};
    }

    try {
        let res = Date.parse(x);
        if (!_.isNaN(res)) {
            return new Date(res);
        }
    } catch (e) {};

    return null;
}

const renderToolTip = (props, params) => (
    <Tooltip show={params.description} {...props}>
       {params.description}
     </Tooltip>
);

export default class VeloForm extends React.Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    state = {
        isUTC: true,

        // A Date() object that is parsed from value in local time.
        timestamp: null,
        multichoices: undefined,
        artifact_loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
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
        this.source = axios.CancelToken.source();

        // Load all artifacts, but only keep the ones that match the
        // specified type
        api.post("v1/GetArtifacts",
                {
                    search_term: "...",
                    type: artifact_type,

                    // No field type: We want the list of sources too

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
                console.log(e);
            }
        }
        this.props.setValue(value);
    }

    render() {
        let param = this.props.param;
        let name = param.friendly_name || param.name;

        switch(param.type) {
        case "hidden":
            return <></>;

        case "csv":
            let data = parseCSV(this.props.value);
            let columns = [{
                dataField: "_id",
                text: "",
                style: {
                    width: '8%',
                },
                headerFormatter: (column, colIndex) => {
                    if (colIndex === 0) {
                        return <ButtonGroup>
                                 <Button variant="default-outline" size="sm"
                                    onClick={() => {
                                        // Add an extra row at the current row index.
                                        let data = parseCSV(this.props.value);
                                        data.data.splice(0, 0, {});
                                        this.props.setValue(
                                            serializeCSV(data.data,
                                                         data.columns));
                                    }}
                                 >
                                   <FontAwesomeIcon icon="plus"/>
                                 </Button>
                               </ButtonGroup>;
                    };
                    return column;
                },
                formatter: (id, row) => {
                    return <ButtonGroup>
                             <Button variant="default-outline" size="sm"
                                     onClick={() => {
                                         // Add an extra row at the current row index.
                                         let data = parseCSV(this.props.value);
                                         data.data.splice(id, 0, {});
                                         this.props.setValue(
                                             serializeCSV(data.data,
                                                          data.columns));
                                     }}
                             >
                               <FontAwesomeIcon icon="plus"/>
                             </Button>
                             <Button variant="default-outline" size="sm"
                                     onClick={() => {
                                         // Drop th current row at the current row index.
                                         let data = parseCSV(this.props.value);
                                         data.data.splice(id, 1);
                                         this.props.setValue(
                                             serializeCSV(data.data,
                                                          data.columns));
                                     }}
                             >
                               <FontAwesomeIcon icon="trash"/>
                             </Button>
                           </ButtonGroup>;
                },
            }];
            _.each(data.columns, (name) => {
                columns.push({dataField: name,
                               editor: {
                                   type: Type.TEXTAREA
                               },
                              text: name});
            });

            _.map(data.data, (item, idx) => {item["_id"] = idx;});

            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>

                  <Col sm="8">
                    <BootstrapTable
                      hover condensed bootstrap4
                      data={data.data}
                      keyField="_id"
                      columns={columns}
                      cellEdit={ cellEditFactory({
                          mode: 'click',
                          afterSaveCell: (oldValue, newValue, row, column) => {
                              // Update the CSV value.
                              let new_data = serializeCSV(data.data, data.columns);
                              this.props.setValue(new_data);
                          },
                          blurToSave: true,
                      }) }
                    />
                  </Col>
                </Form.Group>
            );

        case "regex":
            return (
                  <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                    <Col sm="8">
                      <RegEx
                        value={this.props.value}
                        setValue={this.props.setValue}
                      />
                  </Col>
                </Form.Group>
            );

        case "yara":
            return (
                  <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                    <Col sm="8">
                      <YaraEditor
                        value={this.props.value}
                        setValue={this.props.setValue}
                      />
                  </Col>
                </Form.Group>
            );

        case "timestamp":
            // value prop is always a string in ISO format in UTC timezone.
            let date = convertToDate(this.props.value);

            // Internally the date selector always uses local browser
            // time. If the form is configured to use utc mode, then
            // we convert the UTC time to the equivalent time in local
            // just for the form.
            if(_.isDate(date) && this.state.isUTC) {
                date = localTimeFromUTCTime(date);
            }

            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <DateTimePicker
                      showLeadingZeros={true}
                      onChange={(value) => {
                          // Clear the prop value
                          if (!_.isDate(value)) {
                              this.props.setValue(undefined);

                              // If the form is in UTC we take the
                              // date the form gives us (which is in
                              // local timezone) and force the same
                              // date into a serialized ISO in Z time.
                          } else if(this.state.isUTC) {
                              let local_time = convertToDate(value);
                              let utc_time = utcTimeFromLocalTime(local_time);
                              this.props.setValue(utc_time.toISOString());

                          } else {
                              // When in local time we just set the
                              // time as it is.
                              let local_time = convertToDate(value);
                              this.props.setValue(local_time.toISOString());
                          }
                      }}
                      value={date}
                    />
                    {this.state.isUTC ?
                     <Button variant="default-outline"
                             onClick={() => this.setState({isUTC: false})}
                             size="sm">UTC</Button>:
                     <Button variant="default-outline"
                             onClick={() => this.setState({isUTC: true})}
                             size="sm">Local</Button>
                    }
                  </Col>
                </Form.Group>
            );

        case "choices":
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
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
        case "artifactset":
            // No artifacts means we haven't loaded yet.  If there are truly no artifacts, we've got bigger problems.
            if (this.state.multichoices === undefined ||
                this.state.artifact_loading) {
              return <></>;
            }
            if (Object.keys(this.state.multichoices).length === 0) {
                return (
                  <Form.Group as={Row}>
                    <Alert variant="danger">
                      Warning: No artifacts found for type {
                          this.props.param.artifact_type
                      }.
                    </Alert>
                  </Form.Group>
                );
            }
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                      { _.map(Object.keys(this.state.multichoices), (key, idx) => {
                        return (
                            <OverlayTrigger
                              key={key}
                              delay={{show: 250, hide: 400}}
                              overlay={(props)=>renderToolTip(props, this.state.multichoices[key])}>
                              <div>
                                <Form.Switch label={key} id={key} checked={this.state.multichoices[key].enabled} onChange={this.setMulti.bind(this, key)} />
                              </div>
                            </OverlayTrigger>
                        );
                      })}
                  </Col>
                </Form.Group>
            );

        case "bool":
            return (
                <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
                  </Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="checkbox"
                      label={param.description}
                      onChange={(e) => {
                          if (e.currentTarget.checked) {
                              this.props.setValue("Y");
                          } else {
                              this.props.setValue("N");
                          }
                      }}
                      checked={this.props.value === "Y"}
                      value={this.props.value} />
                  </Col>
                </Form.Group>
            );

        case "upload":
            return <UploadFileForm
                     param={this.props.param}
                     value={this.props.value}
                     setValue={this.props.setValue}
                   />;
        default:
            return (
                  <Form.Group as={Row}>
                  <Form.Label column sm="3">
                    <OverlayTrigger
                      delay={{show: 250, hide: 400}}
                      overlay={(props)=>renderToolTip(props, param)}>
                      <div>
                        {name}
                      </div>
                    </OverlayTrigger>
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
