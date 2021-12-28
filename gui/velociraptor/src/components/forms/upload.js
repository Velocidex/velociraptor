import _ from 'lodash';
import React, { Component } from 'react';
import PropTypes from 'prop-types';
import api from '../core/api-service.js';
import Form from 'react-bootstrap/Form';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import InputGroup from 'react-bootstrap/InputGroup';
import classNames from "classnames";
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Tooltip from 'react-bootstrap/Tooltip';


const renderToolTip = (props, params) => (
    <Tooltip show={params.description} {...props}>
       {params.description}
     </Tooltip>
);

export default class UploadFileForm extends Component {
    static propTypes = {
        param: PropTypes.object,
        value: PropTypes.string,

        // The value contains the URL
        setValue: PropTypes.func.isRequired,
    };

    state = {
        upload: {},
        upload_info: {},
        upload_mode: true,
    }

    componentDidMount = () => {
        if (this.props.param.type === "upload" && this.props.value) {
            let url = new URL(this.props.value);
            let parts = decodeURI(url.pathname).split("/");
            this.setState({upload_info: {
                url: this.props.value,
                filename: parts[parts.length-1],
            }});
        }
    }

    uploadFile = () => {
        if (!this.state.upload.name) {
            return;
        }

        this.setState({loading: true});
        api.upload(
            "v1/UploadFormFile",
            {file: this.state.upload},
            this.props.param).then(response => {
                let url = response.data.url;
                this.props.setValue(url);

                this.setState({loading:false,
                               upload: {},
                               upload_info: response.data});
            }).catch(response=>{
                return this.setState({loading:false, upload_info: {}});
            });
    }

    renderUploadMode = () => {
        let param = this.props.param;
        let name = param.friendly_name || param.name;

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
                <InputGroup className="mb-3">
                  <InputGroup.Prepend>
                    <InputGroup.Text
                      as="button"
                      className="btn btn-default"
                      onClick={()=>{
                          this.setState({upload_mode: !this.state.upload_mode});
                      }}>
                      <FontAwesomeIcon icon="cloud" />
                    </InputGroup.Text>
                    <InputGroup.Text
                      as="button"
                      className={classNames({
                          "btn": true,
                          "btn-default": true,
                          "disabled": !this.state.upload.name,
                      })}
                      disabled={!this.state.upload.name}
                      onClick={this.uploadFile}>
                      { this.state.loading ?
                        <FontAwesomeIcon icon="spinner" spin /> :
                        "Upload"
                      }
                    </InputGroup.Text>
                  </InputGroup.Prepend>
                  <Form.File custom>
                    <Form.File.Input
                      onChange={e => {
                          if (!_.isEmpty(e.currentTarget.files)) {
                              this.setState({
                                  upload_info: {},
                                  upload: e.currentTarget.files[0],
                              });
                          }
                      }}
                    />
                    { this.state.upload_info.filename ?
                      <Form.File.Label data-browse="Select a different file">
                        <a href={ this.state.upload_info.url }>
                          { this.state.upload_info.filename }
                        </a>
                      </Form.File.Label>:
                      <Form.File.Label data-browse="Select file">
                        { this.state.upload.name ?
                          this.state.upload.name:
                          "Click to upload file"}
                      </Form.File.Label>
                    }
                  </Form.File>
                </InputGroup>
              </Col>
            </Form.Group>
        );
    }

    renderURLMode = () => {
        let param = this.props.param;
        let name = param.friendly_name || param.name;

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
                <InputGroup className="mb-3">
                  <InputGroup.Prepend>
                    <InputGroup.Text
                      as="button"
                      className="btn btn-default"
                      onClick={()=>{
                          this.setState({upload_mode: !this.state.upload_mode});
                      }}>
                      <FontAwesomeIcon icon="upload" />
                    </InputGroup.Text>
                  </InputGroup.Prepend>
                  <Form.Control as="textarea"
                                rows={1}
                                placeholder="Type a URL "
                                onChange={(e) => {
                                    this.props.setValue(e.currentTarget.value);
                                }}
                                value={this.props.value} />
                </InputGroup>
              </Col>
            </Form.Group>
        );
    }

    render() {
        if (this.state.upload_mode) {
            return this.renderUploadMode();
        }

        return this.renderURLMode();
    };
}
