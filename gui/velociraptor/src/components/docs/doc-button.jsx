import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';

import api from '../core/api-service.jsx';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import T from '../i8n/i8n.jsx';
import {CancelToken} from 'axios';
import Alert from 'react-bootstrap/Alert';
import Accordion from 'react-bootstrap/Accordion';
import Badge from 'react-bootstrap/Badge';
import parseHTML from '../core/sanitize.jsx';
import { cleanupHTML } from '../core/sanitize.jsx';
import hljs from 'highlight.js';
import ToolViewer from '../tools/tool-viewer.jsx';
import markdownit from 'markdown-it';
import { TablePaginationControl } from '../core/paged-table.jsx';
import InputGroup from 'react-bootstrap/InputGroup';
import Image from './image.jsx';
import { domToReact } from 'html-react-parser';
import Card from 'react-bootstrap/Card';
import { JSONparse } from '../utils/json_parse.jsx';

import "./docs.css";


class Admonition extends Component {
    static propTypes = {
        type: PropTypes.string,
        caption: PropTypes.string,
    }

    renderType = ()=>{
        switch(this.props.type) {
        case "tip":
            return <FontAwesomeIcon icon="lightbulb" />;
        case "note":
        case "info":
            return <FontAwesomeIcon icon="note-sticky" />;
        case "warning":
            return <FontAwesomeIcon icon="bomb" />;
        default:
            return <></>;
        }
    }

    render() {
        return (
            <Card className="admonition">
              <Card.Header>
                <div className="admonition-icon">{ this.renderType() }</div>
                <div className="caption">{ this.props.caption }</div>
              </Card.Header>
              <Card.Body>
                <Card.Text>
                  { this.props.children }
                </Card.Text>
              </Card.Body>
            </Card>
        );
    }
}


class HelpDialog extends Component {
    static propTypes = {
        onClose: PropTypes.func.isRequired,
    }

    state = {
        query: "",
        hits: [],
        error: "",
        total_size: 0,
        start: 0,
        page_size: 10,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount = () => {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if(prevState.query != this.state.query ||
           prevState.start != this.state.start ||
           prevState.page_size != this.state.page_size) {
            this.updateSearch();
        }
    }


    updateSearch = ()=>{
        api.get("v1/SearchDocs", {
            query: this.state.query,
            start: this.state.start,
            length: this.state.page_size,
        }, this.source.token).then(response=>{
            if (response.cancel)
                return;

            let data = response && response.data;
            let items = data && data.Items;
            let total_size = data && data.total;
            this.setState({error: "",
                           total_size: parseInt(total_size || 0) ,
                           hits: items || []});
        }).catch(response=>{
            let error = response && response.response &&
                response.response.data && response.response.data.message;
            this.setState({error: error});
            return false;
        });
    }

    highlight = (text, highlights)=>{
        if(!highlights) {
            return text;
        }

        // The highlights are given in bytes so we need to decode from
        // UTF8, add slices and re-encode. Otherwise multi-byte UTF8
        // sequences will screw up the alignment.
        const encoder = new TextEncoder();
        const decoder = new TextDecoder();

        text = encoder.encode(text);

        let res = [];
        let offset = 0;
        let text_len = text.length;
        for(let i=0;i<highlights.length;i++) {
            let h = highlights[i];
            let start = parseInt(h.start);
            let end = parseInt(h.end);

            if(start > text_len) {
                break;
            }

            if(start > offset) {
                res.push(decoder.decode(text.slice(offset, start)));
            }

            res.push("<span class='highlight'>" +
                     decoder.decode(text.slice(start, end)) +
                     "</span>");
            offset = end;
        }

        if(offset < text_len) {
            res.push(decoder.decode(text.slice(offset, text_len)));
        }

        return res.join("");
    }

    searchTag = tag=>{
        this.setState({query: "+tags:" + tag + " " + this.state.query});
    }


    renderTags = tags=>{
        return _.map(tags, (x, idx)=>{
            return <Badge
                     onClick={()=>this.searchTag(x)}
                     className="tag"
                     key={idx}>{x}</Badge>;
        });
    }

    sanitize = text=>{
        return parseHTML(text, {
            replace: domNode=>{
                if(domNode.name === "img") {
                    let attr = domNode.attribs || {};
                    return <Image alt={attr.alt} src={attr.src} />;
                }

                // Make sure images are opened in a new tab.
                if(domNode.name === "a") {
                    domNode.attribs.target = "_blank";
                    domNode.attribs.rel = "noopener noreferrer";
                    return domNode;
                }

                if(domNode.name === "velo-admonition") {
                    let attr = domNode.attribs || {};
                    return <Admonition type={attr.adtype} caption={attr.caption} >
                             { domToReact(domNode.children) }
                           </Admonition>;
                }

                return domNode;
            }});
    }

    render() {
        const md = markdownit({
            html: true,
            linkify: false,
            highlight: function (str, lang) {
                if (lang && hljs.getLanguage(lang)) {
                    try {
                        return hljs.highlight(str, { language: lang }).value;
                    } catch (__) {}
                }

                return ''; // use external default escaping
            }
        });

        return (
            <Modal show={true}
                   dialogClassName="modal-70w"
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Search Documentation")}</Modal.Title>
              </Modal.Header>
              <Modal.Body className="search-hit">
                <Form>
                  <InputGroup>
                      <Form.Control
                        spellCheck="false"
                        value={this.state.query}
                        onChange={e=>this.setState({query: e.target.value})}
                        as="input"
                        placeholder={T("Search Docs")} />
                      <TablePaginationControl
                        total_size={this.state.total_size}
                        start_row={this.state.start}
                        page_size={this.state.page_size}
                        current_page={this.state.start_row / this.state.page_size}
                        onRowChange={row_offset=>{
                            this.setState({start: row_offset});
                        }}
                        onPageSizeChange={size=>this.setState({page_size: size})}
                      />
                  </InputGroup>
                </Form>
                <Accordion>
                  {_.map(this.state.hits, (v, idx)=>{
                      let full_text = this.highlight(v.full_text, v.highlights);
                      full_text = md.render(full_text || "");

                      let crumbs = JSONparse(v.crumbs) || [];

                      return <Accordion.Item eventKey={idx} key={idx}>
                               <Accordion.Header>
                                 <Row className="hit-row">
                                   <Col sm="2">{this.renderTags(v.tags)}</Col>
                                   <Col sm="10">
                                     { _.map(crumbs, (c, idx)=>{
                                         return (
                                             <>
                                               <a target="_blank"
                                                  className="breadcrumb"
                                                   href={c.url}
                                                   key={idx}>
                                                  { c.name }
                                               </a>
                                               <div className="breadcrumb-divider"/>
                                             </>
                                         );
                                     })}
                                     <a target="_blank" href={v.link}>
                                       {v.title}
                                     </a>
                                   </Col>
                                 </Row>
                               </Accordion.Header>
                               <Accordion.Body className="hit-body">
                                 {this.sanitize(full_text)}
                               </Accordion.Body>
                             </Accordion.Item>;
                  })}
                </Accordion>
              </Modal.Body>
              <Modal.Footer>
                { this.state.error &&
                  <Alert className="doc-error-bar"
                         variant="warning">{this.state.error}</Alert>
                }
                <ToolViewer key="3" name="DocsIndex" />
                <Button variant="secondary"
                        onClick={()=>this.props.onClose()}>
                  {T("Close")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}

export default class DocButton extends Component {
    state = {
        show_dialog: false,
    }

    render() {
        return (
            <>
              <Button
                variant="primary"
                className="help-button"
                onClick={()=>this.setState({show_dialog: true})}
              >
                <FontAwesomeIcon icon="circle-question" />
              </Button>
              { this.state.show_dialog &&
                <HelpDialog
                  onClose={()=>this.setState({show_dialog: false})}
                /> }
            </>
        );
    }
}
