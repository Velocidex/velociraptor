import _ from 'lodash';
import React, { PureComponent, Component } from 'react';
import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from 'react-bootstrap/Button';
import "./json.css";
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import BootstrapTable from 'react-bootstrap-table-next';
import filterFactory from 'react-bootstrap-table2-filter';
import { formatColumns } from "../core/table.jsx";
import VeloTable from '../core/table.jsx';

const scale = 5;
const collapse_string_length = 50;
const collapse_array_length = 5;

class RenderString extends Component {
    static propTypes = {
        value: PropTypes.any,
        collapsed: PropTypes.bool,
    };

    state = {
        expanded: false,
    }

    render() {
        let value = this.props.value || "";
        if (value.length > collapse_string_length) {
            let ellipsis = "";
            if (!this.state.expanded) {
                value = value.substring(0, collapse_string_length);
                ellipsis = <FontAwesomeIcon icon="ellipsis"/>;
            }
            return <a onClick={x=>this.setState({expanded: !this.state.expanded})}>
                     <span className="json-string">"{value}" {ellipsis}</span>
                   </a>;
        }

        return <span className="json-string">"{this.props.value}"</span>;
    }
}


class RenderNumber extends Component {
    static propTypes = {
        value: PropTypes.any,
    };

    render() {
        return <span className="json-number">{this.props.value}</span>;
    }
}


class RenderObject extends Component {
    static propTypes = {
        key_item: PropTypes.string,
        value: PropTypes.any,
        collapsed: PropTypes.bool,
        indent: PropTypes.number,
        trailingComponents: PropTypes.array,
    };

    componentDidMount = () => {
        this.setState({expanded: !this.props.collapsed});
    }

    state = {
        expanded: false,
        open_symbol: "{",
        close_symbol: "}",
    }

    renderExpanded() {
        let indent = this.props.indent || 0;
        let key = this.props.key_item || "";
        if (key) {
            if (isNumeric(key)) {
                key = key + ': ';
            } else {
                key = '"' + key + '": ';
            }
        }

        let opener = <>
                       <span className="folder-icon">
                         <FontAwesomeIcon icon="chevron-down"/>
                       </span> <span className="json-opener-key">{ key }</span>
                       <span className="json-opener">{this.state.open_symbol} </span>
                     </>;

        let elements = [];
        let i = 0;
        let pad = <span className="json-pad" style={{paddingLeft: indent}}/>;
        let pad_elements = <span className="json-pad" style={{
            paddingLeft: indent + 3 * scale}}/>;

        _.forOwn(this.props.value, (v , k)=>{
            i = i + 10;
            if (_.isArray(v)) {
                elements.push(<div key={i}>
                                <RenderArray
                                  value={v}
                                  key_item={k}
                                  collapsed={this.props.collapsed}
                                  indent={indent  + 3 * scale}/>
                              </div>);

            } else if (_.isObject(v)) {
                elements.push(<div key={i}>
                                <RenderObject
                                  value={v}
                                  key_item={k}
                                  collapsed={this.props.collapsed}
                                  indent={indent  + 3 * scale}/>
                              </div>);

            } else if (_.isString(k)) {
                let classes = "json-index";
                if (!isNumeric(k)) {
                    k = '"' + k + '"';
                    classes = "json-key";
                }

                elements.push(<div key={i}>
                                { pad_elements }
                                <span className={classes}>{ k }</span>:
                                <JsonView value={v}
                                          collapsed={this.props.collapsed}/>
                              </div>);
            }
        });

        elements = _.concat(elements, this.props.trailingComponents || []);

        return <>
                 <a onClick={x=>this.setState({expanded:!this.state.expanded})}>
                   { pad } { opener }
                 </a>
                 { elements }
                 <div>{ pad } <span className="json-closer">
                       {this.state.close_symbol}
                     </span></div>
               </>;
    }

    renderCollapsed() {
        let indent = this.props.indent || 0;
        let pad = <span className="variable-row" style={{paddingLeft: indent}}/>;
        let key = this.props.key_item || "";
        if (key) {
            if (isNumeric(key)) {
                key = key + ': ';
            } else {
                key = '"' + key + '": ';
            }
        }

        let opener = <>
                       <span className="folder-icon">
                         <FontAwesomeIcon icon="chevron-right"/>
                       </span> <span className="folder-icon">{ key }</span>
                       <span className="json-opener">{this.state.open_symbol}
                         <FontAwesomeIcon icon="ellipsis"/>
                         <span className="json-closer">
                           {this.state.close_symbol}
                         </span></span>
                     </>;


        return <a onClick={x=>this.setState({expanded:!this.state.expanded})}>
                 { pad } { opener }
               </a>;
    }

    render() {
        if (this.state.expanded) {
            return this.renderExpanded();
        }
        return this.renderCollapsed();
    }
}

class RenderArrayModal extends PureComponent {
    static propTypes = {
        value: PropTypes.array,
        onClose: PropTypes.func.isRequired,
    };

    // Build the data from the value array
    componentDidMount = () => {
        this.calculateTable();
    }

    calculateTable = ()=>{
        let column_names = {};
        let data = [];
        _.each(this.props.value, (x, idx)=>{
            let row = {};
            if (_.isString(x) || isNumeric(x) || _.isEmpty(x)) {
                x={value: x};
            }

            _.forOwn(x, (v, k)=>{
                column_names[k] = true;
                row[k] = v;
            });
            data.push(row);
        });

        let columns = formatColumns(_.map(column_names, (v, x)=>{
            return {dataField: x, text: x, sort: true, filtered: true};
        }));
        this.setState({columns: columns,
                       column_names: _.map(column_names, (v, x)=>x),
                       data: data});
    }

    state = {
        data: [],
        columns: [],
        column_names: [],
        headers: {},
    }


    render() {
        if (_.isEmpty(this.state.data)) {
            return <></>;
        }
        let column_renderers = {};
        _.each(this.state.columns, x=>{
            column_renderers[x.text] = x;
        });

        return <Modal show={true}
                      enforceFocus={true}
                      scrollable={false}
                      size="lg"
                      dialogClassName="modal-90w"
                      onHide={this.props.onClose}>
                 <Modal.Body className="json-array-viewer">
                   <VeloTable rows={this.state.data}
                              column_renderers={column_renderers}/>
                 </Modal.Body>
               </Modal>;
    }
}

class RenderArray extends RenderObject {
    state = {
        expanded: false,
        open_symbol: "[",
        close_symbol: "]",
    }


    render() {
        if (this.props.value &&
            this.props.value.length > collapse_array_length &&
            this.state.expanded) {
            let abridged = this.props.value.slice(0, collapse_array_length-1);
            let indent = this.props.indent || 0;
            let buttons = [
                <a className="json-expand-button"
                   onClick={x=>this.setState({showModal: true})}>
                  <span className="json-pad json-expand-button" style={{
                      paddingLeft: indent + 3 * scale}}>
                    <FontAwesomeIcon icon="ellipsis"/>&nbsp;
                    { this.props.value.length } {T("Total Rows")}
                  </span>
                </a>];

            return <>
                     <RenderArray
                       value={abridged}
                       key_item={this.props.key_item}
                       indent={this.props.indent}
                       collapsed={this.props.collapsed}
                       trailingComponents={buttons}
                     />
                     { this.state.showModal &&
                       <RenderArrayModal
                         value={this.props.value}
                         onClose={x=>this.setState({showModal: false})}
                       />}

                   </>;
        }

        if (this.state.expanded) {
            return this.renderExpanded();
        }
        return this.renderCollapsed();
    }
}

export default class JsonView extends PureComponent {
    static propTypes = {
        value: PropTypes.any,
        collapsed: PropTypes.bool,
        indent: PropTypes.number,
    };

    render() {
        let res = [];
        let pad = <span className="json-pad json-string"
           style={{paddingLeft: this.props.indent}}/>;

        if (_.isArray(this.props.value)) {
            res = <RenderArray value={this.props.value}
                               indent={this.props.indent}
                               collapsed={this.props.collapsed} />;

        } else if (_.isObject(this.props.value)) {
            res = <RenderObject value={this.props.value}
                                indent={this.props.indent}
                                collapsed={this.props.collapsed} />;

        } else if (_.isString(this.props.value)) {
            res = <RenderString value={this.props.value} />;

        } else if (_.isNumber(this.props.value)) {
            res = <RenderNumber value={this.props.value} />;
        }

        return <>
                 { pad }
                 { res }
               </>;
    }
}

function isNumeric(n) {
    return !isNaN(parseFloat(n)) && isFinite(n);
}
